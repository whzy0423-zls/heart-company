package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

var (
	errSecretRotateInvalidKey = errors.New("xinzhili secret rotate invalid key")
	errSecretRotateDatabase   = errors.New("xinzhili secret rotate database error")
)

func main() {
	if err := runSecretRotate(os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runSecretRotate(args []string, getenv func(string) string, out io.Writer) error {
	fs := flag.NewFlagSet("xinzhili-secret-rotate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	oldKeyEnv := fs.String("old-key-env", "XINZHILI_OLD_SECRET_KEY", "environment variable holding old base64 32-byte key")
	newKeyEnv := fs.String("new-key-env", "XINZHILI_NEW_SECRET_KEY", "environment variable holding new base64 32-byte key")
	databaseURLEnv := fs.String("database-url-env", "DATABASE_URL", "environment variable holding PostgreSQL DSN")
	dryRun := fs.Bool("dry-run", false, "validate and count only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	oldCodec, err := xinzhili.NewVoiceSecretCodec(getenv(*oldKeyEnv))
	if err != nil {
		return fmt.Errorf("%w: old key env %s: %v", errSecretRotateInvalidKey, *oldKeyEnv, err)
	}
	newCodec, err := xinzhili.NewVoiceSecretCodec(getenv(*newKeyEnv))
	if err != nil {
		return fmt.Errorf("%w: new key env %s: %v", errSecretRotateInvalidKey, *newKeyEnv, err)
	}
	if *dryRun && strings.TrimSpace(getenv(*databaseURLEnv)) == "" {
		fmt.Fprintln(out, "dry-run：密钥校验通过，未配置数据库连接，未扫描数据")
		_ = oldCodec
		_ = newCodec
		return nil
	}
	dsn := strings.TrimSpace(getenv(*databaseURLEnv))
	if dsn == "" {
		return fmt.Errorf("%w: %s 不能为空", errSecretRotateDatabase, *databaseURLEnv)
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := rotateSecrets(ctx, db, oldCodec, newCodec, *dryRun, out); err != nil {
		return err
	}
	return nil
}

func rotateSecrets(ctx context.Context, db *sql.DB, oldCodec, newCodec *xinzhili.VoiceSecretCodec, dryRun bool, out io.Writer) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT id, api_key_ciphertext FROM app_xinzhili_voice_configs WHERE provider=$1 AND api_key_ciphertext<>'' FOR UPDATE`, xinzhili.TTSProviderAliyunCosyVoice)
	if err != nil {
		return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
	}
	defer rows.Close()
	type item struct {
		id         int64
		ciphertext string
	}
	items := []item{}
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.ciphertext); err != nil {
			return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
	}
	for _, it := range items {
		plaintext, err := oldCodec.Decrypt(it.ciphertext)
		if err != nil {
			return fmt.Errorf("解密音色配置 id=%d 失败: %w", it.id, err)
		}
		if dryRun {
			continue
		}
		rotated, err := newCodec.Encrypt(plaintext)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app_xinzhili_voice_configs SET api_key_ciphertext=$2, update_time=now() WHERE id=$1`, it.id, rotated); err != nil {
			return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
		}
	}
	if dryRun {
		fmt.Fprintf(out, "dry-run：密钥校验通过，已扫描 %d 条阿里音色配置，未写入\n", len(items))
		return tx.Rollback()
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", errSecretRotateDatabase, err)
	}
	fmt.Fprintf(out, "密钥轮换完成：已更新 %d 条阿里音色配置\n", len(items))
	return nil
}
