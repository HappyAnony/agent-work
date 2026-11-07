package server

import (
	"context"
	"strings"

	"wiki-kernel/internal/wikiops"
)

func devicePreview(ctx context.Context, config Config) (context.Context, error) {
	if config.PreviewFile == "" {
		return ctx, nil
	}
	raw, err := privateDeviceFile(config.Root, config.PreviewFile)
	if err != nil {
		return ctx, wikiops.ErrArgument
	}
	var q struct {
		StoreID             string
		Origins             []string
		Widgets             map[string]string
		WidgetScripts       map[string]map[string]string
		InteractiveFrames   []string
		FrameScripts        map[string]map[string]string
		FrameAssets         map[string]map[string]string
		FrameForms          map[string]wikiops.FrameFormPolicy
		FrameSessions       map[string]map[string]string
		Plugins             map[string]string
		PluginStorage       []string
		PluginHotkeys       map[string]map[string]string
		PluginSystemHotkeys map[string]map[string]string
	}
	if err = wikiops.Decode(strings.NewReader(raw), &q); err != nil || q.StoreID == "" {
		return ctx, wikiops.ErrArgument
	}
	if q.StoreID != config.StoreID {
		return ctx, nil
	}
	ctx, err = wikiops.WithExternalPreviewPolicy(ctx, q.Origins, q.Widgets, q.WidgetScripts, q.InteractiveFrames, q.Plugins, q.FrameScripts)
	if err != nil {
		return ctx, err
	}
	ctx, err = wikiops.WithScopedPluginStorage(ctx, q.PluginStorage)
	if err != nil {
		return ctx, err
	}
	ctx, err = wikiops.WithScopedPluginHotkeys(ctx, q.PluginHotkeys)
	if err != nil {
		return ctx, err
	}
	ctx, err = wikiops.WithScopedSystemHotkeys(ctx, q.PluginSystemHotkeys)
	if err != nil {
		return ctx, err
	}
	ctx, err = wikiops.WithFrameResources(ctx, q.FrameAssets, q.FrameForms)
	if err != nil {
		return ctx, err
	}
	return wikiops.WithFrameSessions(ctx, q.FrameSessions)
}
