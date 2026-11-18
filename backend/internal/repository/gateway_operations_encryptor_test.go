package repository

import (
	"context"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/securitysecret"
	"github.com/stretchr/testify/require"
)

func TestGatewayOperationsEncryptorPersistsAcrossDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "wallet-secrets.db") + "?_pragma=foreign_keys(1)"
	open := func() *ent.Client {
		db, err := sql.Open("sqlite", dsn)
		require.NoError(t, err)
		client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
		t.Cleanup(func() { _ = client.Close() })
		return client
	}
	firstClient := open()
	require.NoError(t, firstClient.Schema.Create(ctx))
	first, err := NewGatewayOperationsEncryptor(firstClient)
	require.NoError(t, err)
	ciphertext, err := first.Encrypt("wallet-readonly-token")
	require.NoError(t, err)
	require.NotContains(t, ciphertext, "wallet-readonly-token")
	stored, err := firstClient.SecuritySecret.Query().Where(securitysecret.KeyEQ(gatewayOperationsEncryptionKey)).Only(ctx)
	require.NoError(t, err)
	key, err := hex.DecodeString(stored.Value)
	require.NoError(t, err)
	require.Len(t, key, 32)
	require.NoError(t, firstClient.Close())

	// A fresh connection and provider receive no configuration or previous key.
	secondClient := open()
	second, err := NewGatewayOperationsEncryptor(secondClient)
	require.NoError(t, err)
	plaintext, err := second.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, "wallet-readonly-token", plaintext)
	again, err := secondClient.SecuritySecret.Query().Where(securitysecret.KeyEQ(gatewayOperationsEncryptionKey)).Only(ctx)
	require.NoError(t, err)
	require.Equal(t, stored.Value, again.Value)
	count, err := secondClient.SecuritySecret.Query().Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count, "wallet bootstrap must not create or change another secret")
}

func TestGatewayOperationsEncryptorRejectsInvalidPersistedKeyWithoutReplacement(t *testing.T) {
	for name, value := range map[string]string{"invalid_hex": strings.Repeat("z", 64), "wrong_length": strings.Repeat("ab", 24)} {
		t.Run(name, func(t *testing.T) {
			client := newSecuritySecretTestClient(t)
			_, err := client.SecuritySecret.Create().SetKey(gatewayOperationsEncryptionKey).SetValue(value).Save(context.Background())
			require.NoError(t, err)
			_, err = NewGatewayOperationsEncryptor(client)
			require.ErrorContains(t, err, "must be 32 bytes")
			require.NotContains(t, err.Error(), value)
			stored, err := client.SecuritySecret.Query().Where(securitysecret.KeyEQ(gatewayOperationsEncryptionKey)).Only(context.Background())
			require.NoError(t, err)
			require.Equal(t, value, stored.Value)
		})
	}
}
