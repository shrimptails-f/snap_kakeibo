package di

import (
	"time"

	"snap_kakeibo/backend/internal/auth/application"
	"snap_kakeibo/backend/internal/auth/infrastructure"
	"snap_kakeibo/backend/internal/auth/library/settings"
	"snap_kakeibo/backend/internal/auth/library/token"
	libdynamodb "snap_kakeibo/backend/internal/library/dynamodb"
	libssm "snap_kakeibo/backend/internal/library/ssm"
	"snap_kakeibo/backend/internal/library/timewrapper"

	"go.uber.org/dig"
)

// provideAuthTokenDependencies はトークンを発行する認証 Lambda（auth-login / auth-refresh）が共通で使う
// 利用者リポジトリ・JWT 署名鍵・access token 発行・refresh token 生成と永続化を登録する。
func provideAuthTokenDependencies(container *dig.Container, scope string, cfg settings.Config) error {
	return Provide(container, scope,
		func() settings.Config { return cfg },
		func(client *libdynamodb.Client, cfg settings.Config) application.UserRepository {
			return infrastructure.DynamoDBUserRepository{Table: client.Table(cfg.UsersTable)}
		},
		func(cfg settings.Config, client *libssm.Client) token.SecretProvider {
			if cfg.JWTSecret != "" {
				return token.StaticSecretProvider{Value: cfg.JWTSecret}
			}
			return &token.SSMSecretProvider{Parameter: client.Parameter(cfg.JWTSecretParameter)}
		},
		func(secrets token.SecretProvider, cfg settings.Config, clock timewrapper.Interface) application.AccessTokenIssuer {
			return token.JWTIssuer{Secrets: secrets, Issuer: issuer(cfg.Stage), Clock: clock, TTL: 15 * time.Minute}
		},
		func() token.RefreshTokenGenerator { return token.RefreshTokenGenerator{} },
		func(generator token.RefreshTokenGenerator) application.RefreshTokenGenerator { return generator },
		func(client *libdynamodb.Client, cfg settings.Config) application.RefreshTokenRepository {
			return infrastructure.DynamoDBRefreshTokenRepository{Table: client.Table(cfg.RefreshTokensTable)}
		},
	)
}

// provideAccessTokenVerification は access token の検証を行う Lambda(auth-check と認証が必要な API)が共通で使う
// JWT 署名鍵の provider・検証器・認証状態確認ユースケースを登録する。
// 認証 feature の設定型に依存しないよう値で受け取り、他 feature の設定からも組み立てられるようにしている。
func provideAccessTokenVerification(container *dig.Container, scope, jwtSecret, jwtSecretParameter, stage string) error {
	return Provide(container, scope,
		func(client *libssm.Client) token.SecretProvider {
			if jwtSecret != "" {
				return token.StaticSecretProvider{Value: jwtSecret}
			}
			return &token.SSMSecretProvider{Parameter: client.Parameter(jwtSecretParameter)}
		},
		func(secrets token.SecretProvider, clock timewrapper.Interface) application.AccessTokenVerifier {
			return token.JWTVerifier{Secrets: secrets, Issuer: issuer(stage), Clock: clock}
		},
		application.NewCheckUsecase,
	)
}

func issuer(stage string) string {
	if stage == "" {
		return "snap-kakeibo"
	}
	return "snap-kakeibo-" + stage
}
