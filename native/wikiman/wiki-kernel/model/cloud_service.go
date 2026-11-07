// SiYuan - Refactor your thinking
// Copyright (c) 2020-present, b3log.org
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package model

import (
	"context"
	"errors"

	"github.com/88250/gulu"
	"github.com/siyuan-note/logging"
	"wiki-kernel/conf"
)

var ErrFailedToConnectCloudServer = errors.New("failed to connect cloud server")

func CloudChatGPT(msg string, history []string) (string, bool, error) {
	return CloudChatGPTContext(context.Background(), msg, history)
}

func StartFreeTrial() error { return StartFreeTrialContext(context.Background()) }
func DeactivateUser() error { return DeactivateUserContext(context.Background()) }

func SetCloudBlockReminder(id, data string, timed int64) error {
	return SetCloudBlockReminderContext(context.Background(), id, data, timed)
}

func LoadUploadToken() error { _, e := LoadUploadTokenContext(context.Background()); return e }

func RefreshCheckJob() {
	logOwnedTaskError("periodic cloud refresh", RefreshCheckJobContext(context.Background()))
}

func refreshSubscriptionExpirationRemind() {
	if e := Conf.ScheduleSubscriptionReminder(); e != nil {
		logging.LogErrorf("schedule subscription reminder failed")
	}
}

func refreshAnnouncement() {
	if e := Conf.ScheduleAnnouncements(); e != nil {
		logging.LogErrorf("schedule announcement failed")
	}
}

func RefreshUser(token string) {
	if _, e := RefreshUserContext(context.Background(), token); e != nil {
		logging.LogErrorf("cloud account refresh failed")
	}
}

func loadUserFromConf() *conf.User {
	if Conf == nil || Conf.UserData == "" {
		return nil
	}
	u, _ := decodeCloudUser(Conf.UserData, true)
	return u
}

func RemoveCloudShorthands(ids []string) error {
	return RemoveCloudShorthandsContext(context.Background(), ids)
}
func GetCloudShorthand(id string) (map[string]interface{}, error) {
	return GetCloudShorthandContext(context.Background(), id)
}
func GetCloudShorthands(page int) (map[string]interface{}, error) {
	return GetCloudShorthandsContext(context.Background(), page)
}

func UseActivationcode(code string) error {
	return UseActivationcodeContext(context.Background(), code)
}

func CheckActivationcode(code string) (int, string) {
	return CheckActivationcodeContext(context.Background(), code)
}

func Login(userName, password, captcha string, region int) *gulu.Result {
	v, e := LoginContext(context.Background(), userName, password, captcha, region)
	if e != nil {
		return &gulu.Result{Code: -1, Msg: Conf.Language(18)}
	}
	return &gulu.Result{Code: v.Code, Msg: v.Msg, Data: v.Data}
}
func Login2fa(token, code string) (map[string]interface{}, error) {
	return Login2faContext(context.Background(), token, code)
}
func LogoutUser() {
	if e := LogoutUserContext(context.Background()); e != nil {
		logging.LogErrorf("cloud account logout failed")
	}
}
