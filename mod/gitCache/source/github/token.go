// Copyright 2021 Canva Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package github

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

type TokenSource struct {
	AppID          string
	InstallationID string
	PrivateKey     *rsa.PrivateKey

	tokenExpiryDelta time.Duration

	token *oauth2.Token
	mu    sync.Mutex
}

func (ts *TokenSource) Token() (*oauth2.Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.token.Valid() {
		currentTime := time.Now()
		if !ts.token.Expiry.IsZero() && ts.token.Expiry.Round(0).Add(-ts.tokenExpiryDelta).Before(currentTime) {
			log.Printf("Current OAuth token will expired in %s. Will regenerate.\n", ts.token.Expiry.Sub(currentTime))
		} else {
			return ts.token, nil
		}
	} else {
		log.Printf("Current OAuth token is not valid. Will regenerate.\n")
	}

	ts.token = nil

	newTok, err := GenerateOAuthTokenFromApp(ts.AppID, ts.InstallationID, ts.PrivateKey)
	if err == nil {
		ts.token = &newTok
		log.Printf("New OAuth token generated. Will expired at %s\n", ts.token.Expiry)
	} else {
		log.Printf("New OAuth token generation failed. %v\n", err)
	}

	return ts.token, nil
}

func NewTokenSource(appID string, installationID string, privateKey string, tokenExpiryDelta time.Duration) (*TokenSource, error) {
	if appID == "" {
		return nil, fmt.Errorf("github app id must be provided")
	}
	if installationID == "" {
		return nil, fmt.Errorf("github app installation id must be provided")
	}

	if privateKey == "" {
		return nil, fmt.Errorf("github app private key must be provided")
	}

	block, _ := pem.Decode([]byte(privateKey))
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block")
	}
	pk, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	log.Printf("OAuth token will be discarded %s before its expiry\n", tokenExpiryDelta)

	return &TokenSource{
		AppID:            appID,
		InstallationID:   installationID,
		PrivateKey:       pk,
		tokenExpiryDelta: tokenExpiryDelta,
	}, nil
}
