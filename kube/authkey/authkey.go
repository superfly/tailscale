// Copyright (c) Tailscale Inc & contributors
// SPDX-License-Identifier: BSD-3-Clause

//go:build !plan9

// Package authkey provides shared logic for handling auth key reissue
// requests between tailnet clients (containerboot, k8s-proxy) and the
// operator.
//
// When a client fails to authenticate (expired key, single-use key already
// used, device deleted), it signals the operator by setting a marker in its
// state Secret. The operator responds by deleting the old device and issuing
// a new auth key. The client watches for the new key and restarts to apply it.
package authkey

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"tailscale.com/ipn/conffile"
	"tailscale.com/kube/kubeapi"
	"tailscale.com/kube/kubeclient"
	"tailscale.com/kube/kubetypes"
)

const (
	fieldManager           = "tailscale-container"
	kubeletMountedConfigLn = "..data"
)

// SetReissueAuthKey sets the reissue_authkey marker in the state Secret to
// signal to the operator that a new auth key is needed. The marker value is
// the auth key that failed to authenticate.
func SetReissueAuthKey(ctx context.Context, kc kubeclient.Client, stateSecretName string, authKey string) error {
	s := &kubeapi.Secret{
		Data: map[string][]byte{
			kubetypes.KeyReissueAuthkey: []byte(authKey),
		},
	}

	log.Printf("Requesting a new auth key from operator")
	return kc.StrategicMergePatchSecret(ctx, stateSecretName, s, fieldManager)
}

// ClearReissueAuthKey removes the reissue_authkey marker from the state Secret
// to signal to the operator that we've successfully received the new key.
func ClearReissueAuthKey(ctx context.Context, kc kubeclient.Client, stateSecretName string) error {
	s := &kubeapi.Secret{
		Data: map[string][]byte{
			kubetypes.KeyReissueAuthkey: nil,
		},
	}
	return kc.StrategicMergePatchSecret(ctx, stateSecretName, s, fieldManager)
}

// WaitForAuthKeyReissue watches the config file for a new auth key different
// from oldAuthKey. It uses fsnotify when available for fast notification, with
// a polling fallback for logging and reliability. Returns when a new key is
// detected or maxWait expires.
//
// The clearFn callback is called when a new key is detected, to clear the
// reissue marker from the state Secret.
func WaitForAuthKeyReissue(ctx context.Context, configPath string, oldAuthKey string, maxWait time.Duration, clearFn func(context.Context) error) error {
	log.Printf("Waiting for operator to provide new auth key (max wait: %v)", maxWait)

	ctx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	tailscaledCfgDir := filepath.Dir(configPath)
	toWatch := filepath.Join(tailscaledCfgDir, kubeletMountedConfigLn)

	var (
		pollTicker <-chan time.Time
		eventChan  <-chan fsnotify.Event
	)

	pollInterval := 5 * time.Second

	// Try to use fsnotify for faster notification
	if w, err := fsnotify.NewWatcher(); err != nil {
		log.Printf("auth key reissue: fsnotify unavailable, using polling: %v", err)
	} else if err := w.Add(tailscaledCfgDir); err != nil {
		w.Close()
		log.Printf("auth key reissue: fsnotify watch failed, using polling: %v", err)
	} else {
		defer w.Close()
		log.Printf("auth key reissue: watching for config changes via fsnotify")
		eventChan = w.Events
	}

	// still keep polling if using fsnotify, for logging and in case fsnotify fails
	pt := time.NewTicker(pollInterval)
	defer pt.Stop()
	pollTicker = pt.C

	start := time.Now()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for auth key reissue after %v", maxWait)
		case <-pollTicker: // Waits for polling tick, continues when received
		case event := <-eventChan:
			if event.Name != toWatch {
				continue
			}
		}

		newAuthKey := AuthKeyFromConfig(configPath)
		if newAuthKey != "" && newAuthKey != oldAuthKey {
			log.Printf("New auth key received from operator after %v", time.Since(start).Round(time.Second))

			if err := clearFn(ctx); err != nil {
				log.Printf("Warning: failed to clear reissue request: %v", err)
			}

			return nil
		}

		if eventChan == nil && pollTicker != nil {
			log.Printf("Waiting for new auth key from operator (%v elapsed)", time.Since(start).Round(time.Second))
		}
	}
}

// AuthKeyFromConfig extracts the auth key from a tailscaled config file.
// Returns empty string if the file cannot be read or contains no auth key.
func AuthKeyFromConfig(path string) string {
	if cfg, err := conffile.Load(path); err == nil && cfg.Parsed.AuthKey != nil {
		return *cfg.Parsed.AuthKey
	}

	return ""
}
