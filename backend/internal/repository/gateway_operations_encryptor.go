package repository

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const gatewayOperationsEncryptionKey = "gateway_operations_encryption_key"

// NewGatewayOperationsEncryptor keeps wallet credentials decryptable across
// restarts even when the optional TOTP configuration generates a temporary key.
func NewGatewayOperationsEncryptor(client *ent.Client) (service.GatewayOperationsEncryptor, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	secret, _, err := getOrCreateGeneratedSecuritySecret(ctx, client, gatewayOperationsEncryptionKey, 32)
	if err != nil {
		return nil, fmt.Errorf("ensure gateway operations encryption key: %w", err)
	}
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("stored gateway operations encryption key must be 32 bytes (64 hex chars)")
	}
	return &AESEncryptor{key: key}, nil
}
