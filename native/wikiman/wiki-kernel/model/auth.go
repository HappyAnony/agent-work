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
	"crypto/rand"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"wiki-kernel/conf"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/siyuan-note/logging"
)

type Account struct {
	Username string
	Password string
	Token    string
}
type AccountsMap map[string]*Account // username -> account
type SessionsMap map[string]string   // sessionID -> username
type ClaimsKeyType string

const (
	XAuthTokenKey = "X-Auth-Token"

	SessionIdCookieName = "publish-visitor-session-id"

	ClaimsContextKey = "claims"

	iss = "siyuan-publish-reverse-proxy-server"
	sub = "publish"
	aud = "wiki-kernel"

	ClaimsKeyRole string = "role"
)

var (
	accountsMap = AccountsMap{}
	sessionsMap = SessionsMap{}
	sessionLock = sync.RWMutex{}

	jwtKey             []byte
	lastPublishAuth    *conf.BasicAuth
	lastPublishOwner   *AppConf
	lastPublishEnabled *bool
)

func GetBasicAuthAccount(username string) *Account {
	sessionLock.RLock()
	defer sessionLock.RUnlock()
	a := accountsMap[username]
	if a == nil {
		return nil
	}
	copy := *a
	return &copy
}

func GetBasicAuthUsernameBySessionID(sessionID string) string {
	sessionLock.RLock()
	defer sessionLock.RUnlock()
	return sessionsMap[sessionID]
}

func GetNewSessionID() string {
	sessionID := uuid.New().String()
	return sessionID
}

func AddSession(sessionID, username string) {
	sessionLock.Lock()
	defer sessionLock.Unlock()
	sessionsMap[sessionID] = username
}

func DeleteSession(sessionID string) {
	sessionLock.Lock()
	defer sessionLock.Unlock()
	delete(sessionsMap, sessionID)
}

func InitAccounts() {
	if Conf == nil || Conf.Publish == nil {
		logging.LogErrorf("publish configuration unavailable")
		return
	}
	if e := ConfigurePublishOwnerAccounts(Conf, Conf.Publish); e != nil {
		logging.LogErrorf("configure publish accounts failed: %s", e)
	}
}
func InitJWT() {
	if Conf == nil || Conf.Publish == nil {
		return
	}
	if e := configurePublishAccounts(Conf.Publish.Auth, true, Conf, &Conf.Publish.Enable); e != nil {
		logging.LogErrorf("configure publish JWT failed: %s", e)
	}
}

// Identical reads retain keys and sessions. Build a complete replacement before
// publication; getters return copies and JWT verification shares the same lock.
func ConfigurePublishAccounts(auth *conf.BasicAuth) error {
	return configurePublishAccounts(auth, false, nil, nil)
}
func ConfigurePublishOwnerAccounts(owner *AppConf, p *conf.Publish) error {
	if owner == nil || p == nil {
		return errors.New("publish_auth_unbound")
	}
	return configurePublishAccounts(p.Auth, false, owner, &p.Enable)
}

func ClearPublishOwnerAccounts(owner *AppConf) error {
	sessionLock.Lock()
	defer sessionLock.Unlock()
	if lastPublishOwner != owner {
		return nil
	}
	accountsMap = AccountsMap{}
	sessionsMap = SessionsMap{}
	jwtKey = nil
	lastPublishAuth = nil
	lastPublishOwner = nil
	lastPublishEnabled = nil
	return nil
}
func configurePublishAccounts(auth *conf.BasicAuth, force bool, owner *AppConf, enabled *bool) error {
	if auth == nil {
		return errors.New("publish_auth_unbound")
	}
	owned := *auth
	owned.Accounts = nil
	for _, a := range auth.Accounts {
		if a == nil {
			return errors.New("invalid_publish_account")
		}
		v := *a
		owned.Accounts = append(owned.Accounts, &v)
	}
	sessionLock.Lock()
	defer sessionLock.Unlock()
	if !force && reflect.DeepEqual(lastPublishAuth, &owned) && (owner == nil || owner == lastPublishOwner) && (enabled == nil || lastPublishEnabled != nil && *enabled == *lastPublishEnabled) {
		return nil
	}
	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		return e
	}
	accounts := AccountsMap{"": &Account{}}
	for _, a := range owned.Accounts {
		accounts[a.Username] = &Account{Username: a.Username, Password: a.Password}
	}
	for username, a := range accounts {
		t := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"iss": iss, "sub": sub, "aud": aud, "jti": username, ClaimsKeyRole: RoleReader})
		token, e := t.SignedString(key)
		if e != nil {
			return e
		}
		a.Token = token
	}
	accountsMap = accounts
	jwtKey = key
	lastPublishAuth = &owned
	lastPublishOwner = owner
	if enabled != nil {
		v := *enabled
		lastPublishEnabled = &v
	}
	sessionsMap = SessionsMap{}
	return nil
}

func ParseJWT(tokenString string) (*jwt.Token, error) {
	sessionLock.RLock()
	defer sessionLock.RUnlock()
	if len(jwtKey) != 32 {
		return nil, errors.New("publish_auth_unbound")
	}
	// REF: https://golang-jwt.github.io/jwt/usage/parse/
	return jwt.Parse(
		tokenString,
		func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		},
		jwt.WithIssuer(iss),
		jwt.WithSubject(sub),
		jwt.WithAudience(aud),
	)
}

func ParseXAuthToken(r *http.Request) *jwt.Token {
	tokenString := r.Header.Get(XAuthTokenKey)
	if tokenString != "" {
		if token, err := ParseJWT(tokenString); err != nil {
			logging.LogErrorf("JWT parse failed: %s", err)
		} else {
			return token
		}
	}
	return nil
}

func GetTokenClaims(token *jwt.Token) jwt.MapClaims {
	return token.Claims.(jwt.MapClaims)
}

func GetClaimRole(claims jwt.MapClaims) Role {
	if role := claims[ClaimsKeyRole]; role != nil {
		return Role(role.(float64))
	}
	return RoleVisitor
}
