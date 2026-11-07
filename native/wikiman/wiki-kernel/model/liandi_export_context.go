// SPDX-License-Identifier: AGPL-3.0-or-later
package model

import (
	"context"
	"errors"
	"fmt"
	"github.com/88250/gulu"
	"github.com/88250/lute/ast"
	"github.com/88250/lute/editor"
	"github.com/88250/lute/parse"
	"github.com/siyuan-note/httpclient"
	"html"
	"net/http"
	"path"
	"strings"
	"time"
	"wiki-kernel/conf"
	"wiki-kernel/internal/assetflow"
	"wiki-kernel/treenode"
	"wiki-kernel/util"
)

type liandiReply struct {
	Status, Code int
	Message      string
	ID           string
}
type liandiPorts struct {
	request func(context.Context, string, string, string, map[string]interface{}) (liandiReply, error)
	upload  func(context.Context, []string) error
}

func requestLiandiArticle(ctx context.Context, method, address, token string, body map[string]interface{}) (liandiReply, error) {
	result := gulu.Ret.NewResult()
	request := httpclient.NewCloudRequest30s().SetContext(ctx).SetSuccessResult(result).SetCookies(&http.Cookie{Name: "symphony", Value: token})
	if body != nil {
		request.SetBody(body)
	}
	response, err := request.Send(method, address)
	if err != nil {
		return liandiReply{}, err
	}
	if response == nil {
		return liandiReply{}, errors.New("article_response_missing")
	}
	id, _ := result.Data.(string)
	return liandiReply{Status: response.StatusCode, Code: result.Code, Message: result.Msg, ID: id}, ctx.Err()
}

func export2LiandiContextWithPorts(ctx context.Context, id string, ports liandiPorts) error {
	base, err := captureLegacyReindexOwner(ctx)
	if err != nil {
		return err
	}
	if base.owner.persistenceReadOnly {
		return ErrSettingReadOnly
	}
	return base.run(ctx, func(c, life context.Context) error {
		m, e := captureMarkdownExport(c)
		if e != nil {
			return e
		}
		ui, e := captureLegacyDelayedUIOwner(c)
		if e != nil {
			return e
		}
		var cfg conf.AppData
		if e = base.owner.WithSettingSnapshot(c, func(v conf.AppData) error { cfg = v; return base.guard(c) }); e != nil {
			return e
		}
		if cfg.User == nil || cfg.User.UserToken == "" {
			return errors.New("cloud_account_unbound")
		}
		tree, _, e := m.prepareFocusedExport(id)
		if e != nil {
			return e
		}
		if IsUserGuide(tree.Box) {
			return errors.New(ui.language(204, true))
		}
		assets, e := assetflow.LinkDests(tree.Root, m.read.readExportView)
		if e != nil {
			return e
		}
		ast.Walk(tree.Root, func(n *ast.Node, enter bool) ast.WalkStatus {
			if !enter || n.Type != ast.NodeBlockQueryEmbedScript {
				return ast.WalkContinue
			}
			stmt := strings.ReplaceAll(html.UnescapeString(n.TokensStr()), editor.IALValEscNewLine, "\n")
			rows, err := m.selectEmbeddedBlocks(stmt)
			if err != nil {
				e = err
				return ast.WalkStop
			}
			for _, row := range rows {
				t, err := m.embeddedTree(row.ID)
				if err != nil {
					e = err
					return ast.WalkStop
				}
				node := treenode.GetNodeInTree(t, row.ID)
				if node == nil {
					e = ErrBlockNotFound
					return ast.WalkStop
				}
				values, err := assetflow.LinkDests(node, m.read.readExportView)
				if err != nil {
					e = err
					return ast.WalkStop
				}
				assets = append(assets, values...)
			}
			return ast.WalkContinue
		})
		if e != nil {
			return e
		}
		if ports.upload == nil {
			ports.upload = func(c context.Context, paths []string) error {
				_, e := base.owner.uploadCloudAssets(c, paths, bizTypeExport2Liandi)
				return e
			}
		}
		if ports.request == nil {
			ports.request = requestLiandiArticle
		}
		if e = ports.upload(c, gulu.Str.RemoveDuplicatedElem(assets)); e != nil {
			return e
		}
		message := ""
		if !ui.publish(c, func() { message = util.PushMsg(ui.language(182, true), 15000) }) {
			return conf.ErrBusinessUnbound
		}
		defer ui.publish(life, func() { util.PushClearMsg(message) })
		server := util.GetCloudAccountServerForRegion(cfg.CloudRegion)
		article := tree.Root.IALAttr("custom-liandi-articleId")
		found := false
		if article != "" {
			response, e := ports.request(c, http.MethodGet, server+"/api/v2/article/update/"+article, cfg.User.UserToken, nil)
			if e != nil {
				return e
			}
			if response.Status == 200 {
				found = response.Code == 0
			} else if response.Status != 404 {
				return fmt.Errorf("get liandi article info failed [sc=%d]", response.Status)
			}
		}
		title, tags := path.Base(tree.HPath), tree.Root.IALAttr("tags")
		renderer := m.referenceRenderer()
		baseURL := util.GetCloudForumAssetsServerForRegion(cfg.CloudRegion) + time.Now().Format("2006/01") + "/siyuan/" + cfg.User.UserId + "/"
		content := exportMarkdownContent0(id, tree, baseURL, true, false, false, ".md", 3, 1, 1, "#", "#", "", "", false, false, nil, true, false, map[string]*parse.Tree{}, renderer)
		if e = renderer.check(); e != nil {
			return e
		}
		if e = m.guard(); e != nil {
			return e
		}
		method, address := http.MethodPost, server+"/api/v2/article"
		if found {
			method, address = http.MethodPut, address+"/"+article
		}
		response, e := ports.request(c, method, address, cfg.User.UserToken, map[string]interface{}{"articleTitle": title, "articleTags": tags, "articleContent": content})
		if e != nil {
			return e
		}
		if response.Status != 200 {
			return fmt.Errorf("send article to liandi failed [sc=%d]", response.Status)
		}
		if response.Code != 0 {
			return errors.New(response.Message)
		}
		if !found {
			article = response.ID
			if article == "" {
				return errors.New("article_identity_missing")
			}
			if e = SetBlockAttrsContext(c, id, map[string]string{"custom-liandi-articleId": article}); e != nil {
				return e
			}
			if e = m.read.binding.scope.commit.r.queue.Flush(context.WithoutCancel(c)); e != nil {
				return e
			}
		}
		if !ui.publish(c, func() { util.PushMsg(fmt.Sprintf(ui.language(181, true), server+"/article/"+article), 7000) }) {
			return conf.ErrBusinessUnbound
		}
		return nil
	})
}
