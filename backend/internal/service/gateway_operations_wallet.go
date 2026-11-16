package service

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/util/urlvalidator"
)

// Wallet traffic has a separate transport: credentials cannot follow redirects,
// environment proxies or a DNS response pointing at the private network.
func gatewayWalletClient() *http.Client {
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, ResponseHeaderTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			if len(ips) == 0 {
				return nil, errors.New("wallet host has no addresses")
			}
			for _, ip := range ips {
				if ip.IP.IsLoopback() || ip.IP.IsPrivate() || ip.IP.IsLinkLocalUnicast() || ip.IP.IsLinkLocalMulticast() || ip.IP.IsUnspecified() || ip.IP.IsMulticast() {
					return nil, errors.New("wallet host must resolve to public addresses")
				}
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
	}
	return &http.Client{Transport: transport, Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("wallet redirects are disabled") }}
}
func gatewayWalletRead(ctx context.Context, client *http.Client, w *GatewayWallet, token, path string) (map[string]json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, w.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if path != "/api/status" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Accept", "application/json")
	if w.Provider == "newapi" && path != "/api/status" {
		req.Header.Set("New-Api-User", w.RemoteUserID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("wallet connection failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wallet returned HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(b) > 1<<20 {
		return nil, errors.New("invalid wallet response size")
	}
	var envelope struct {
		Code    int                        `json:"code"`
		Success *bool                      `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(b, &envelope) != nil || envelope.Data == nil || envelope.Code != 0 || (envelope.Success != nil && !*envelope.Success) {
		return nil, errors.New("wallet API rejected the read request")
	}
	return envelope.Data, nil
}
func gatewayWalletNumber(data map[string]json.RawMessage, key string) (float64, error) {
	var number *float64
	raw, ok := data[key]
	if !ok {
		return 0, fmt.Errorf("wallet response missing %s", key)
	}
	if err := json.Unmarshal(raw, &number); err == nil && number != nil {
		return *number, nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("wallet response has invalid %s", key)
		}
		return value, nil
	}
	return 0, fmt.Errorf("wallet response has invalid %s", key)
}
func gatewayWalletBalance(provider string, profile, status map[string]json.RawMessage) (float64, string, error) {
	if provider == "sub2api" {
		balance, err := gatewayWalletNumber(profile, "balance")
		return balance, "USD", err
	}
	quota, err := gatewayWalletNumber(profile, "quota")
	if err != nil {
		return 0, "", err
	}
	scale, err := gatewayWalletNumber(status, "quota_per_unit")
	if err != nil || scale <= 0 {
		return 0, "", errors.New("new API quota conversion unavailable")
	}
	var display string
	_ = json.Unmarshal(status["quota_display_type"], &display)
	balance := quota / scale
	switch strings.ToUpper(display) {
	case "USD":
		return balance, "USD", nil
	case "CNY":
		rate, e := gatewayWalletNumber(status, "usd_exchange_rate")
		if e == nil && rate > 0 && rate < 1e6 {
			return balance * rate, "CNY", nil
		}
	}
	return balance, "quota_units", nil
}
func (s *GatewayOperationsService) RefreshWallet(ctx context.Context, id int64) (*GatewayWallet, error) {
	w, err := s.repo.Wallet(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	w.LastAttemptAt = &now
	refreshErr := func() error {
		if _, err := urlvalidator.ValidateHTTPSURL(w.BaseURL, urlvalidator.ValidationOptions{}); err != nil {
			return errors.New("wallet base URL must be public HTTPS")
		}
		parsed, _ := url.Parse(w.BaseURL)
		if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return errors.New("invalid wallet URL")
		}
		token, err := s.encryptor.Decrypt(w.CredentialCipher)
		if err != nil {
			return errors.New("wallet credential cannot be decrypted")
		}
		client := s.walletClient
		path := "/api/v1/user/profile"
		if w.Provider == "newapi" {
			path = "/api/user/self"
		}
		profile, err := gatewayWalletRead(ctx, client, w, token, path)
		if err != nil {
			return err
		}
		// Remote identity prevents duplicate wallets from silently representing another login.
		remoteID, err := gatewayWalletNumber(profile, "id")
		if err != nil || strconv.FormatInt(int64(remoteID), 10) != w.RemoteUserID {
			return errors.New("remote user ID does not match wallet configuration")
		}
		var status map[string]json.RawMessage
		if w.Provider == "newapi" {
			status, err = gatewayWalletRead(ctx, client, w, token, "/api/status")
			if err != nil {
				return err
			}
		}
		balance, currency, err := gatewayWalletBalance(w.Provider, profile, status)
		if err != nil {
			return err
		}
		if currency != w.Currency {
			return fmt.Errorf("wallet currency is %s, configured %s; correct the currency before alerting", currency, w.Currency)
		}
		w.Balance = &balance
		w.RefreshedAt = &now
		return nil
	}()
	w.LastError = ""
	if refreshErr != nil {
		w.LastError = refreshErr.Error()
	}
	w.Stale = refreshErr != nil || w.RefreshedAt == nil || time.Since(*w.RefreshedAt) > 30*time.Minute
	if err = s.repo.SaveWalletObservation(ctx, w); err != nil {
		return nil, err
	}
	if refreshErr == nil && w.Threshold != nil && w.Balance != nil && *w.Balance <= *w.Threshold && w.NotifyEmail != "" {
		claimed, e := s.repo.ClaimWalletNotification(ctx, w.ID, now)
		if e != nil {
			return w, e
		}
		if claimed {
			e = s.notifications.Send(ctx, NotificationEmailSendInput{Event: NotificationEmailEventOpsAlert, Locale: "zh", RecipientEmail: w.NotifyEmail, SourceType: "gateway_wallet", SourceID: strconv.FormatInt(w.ID, 10), ReminderKey: now.Format("2006-01-02"), Variables: map[string]string{"rule_name": "供应商钱包余额 · " + w.Name, "severity": "warning", "alert_status": "firing", "metric_type": "wallet_balance", "operator": "<=", "metric_value": fmt.Sprintf("%.6f %s", *w.Balance, w.Currency), "threshold_value": fmt.Sprintf("%.6f %s", *w.Threshold, w.Currency), "triggered_at": now.Format(time.RFC3339), "alert_description": "供应商共享钱包余额低于阈值；请人工确认并充值。"}})
			if e != nil {
				_ = s.repo.ReleaseWalletNotification(ctx, w.ID, now)
				w.LastError = "balance refreshed; notification delivery failed"
				_ = s.repo.SaveWalletObservation(ctx, w)
				return w, e
			}
		}
	}
	return w, nil
}

// Start launches no implicit work. Every tick and every wallet checks the persisted
// gate, so disabling the worker also stops the remainder of an ongoing batch.
func (s *GatewayOperationsService) Start(ctx context.Context) {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()
	if s.workerCancel != nil {
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.workerCancel = cancel
	s.workerDone = make(chan struct{})
	go func() {
		defer close(s.workerDone)
		ctx := workerCtx
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshDueWallets(ctx)
			}
		}
	}()
}

// Stop cancels in-flight wallet reads and waits for the worker to exit.
func (s *GatewayOperationsService) Stop() {
	s.workerMu.Lock()
	if s.workerCancel == nil {
		s.workerMu.Unlock()
		return
	}
	cancel, done := s.workerCancel, s.workerDone
	s.workerMu.Unlock()
	cancel()
	<-done
	s.walletClient.CloseIdleConnections()
}

func (s *GatewayOperationsService) refreshDueWallets(ctx context.Context) {

	config, err := s.repo.Config(ctx)
	if err != nil || !config.WorkerEnabled {
		return
	}
walletPages:
	for offset := 0; ; offset += 50 {
		wallets, e := s.repo.Wallets(ctx, 50, offset)
		if e != nil {
			break
		}
		for _, w := range wallets {
			current, e := s.repo.Config(ctx)
			if e != nil || !current.WorkerEnabled {
				break walletPages
			}
			if w.LastAttemptAt == nil || time.Since(*w.LastAttemptAt) >= time.Duration(current.RefreshMinutes)*time.Minute {
				_, _ = s.RefreshWallet(ctx, w.ID)
			}
		}
		if len(wallets) < 50 {
			break
		}
	}
}
