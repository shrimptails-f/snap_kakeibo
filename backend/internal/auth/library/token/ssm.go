package token

import (
	"context"
	"fmt"
	"sync"

	libssm "snap_kakeibo/backend/internal/library/ssm"
)

// SSMSecretProvider は SecureString の JWT 署名鍵を一度だけ取得する。
type SSMSecretProvider struct {
	Parameter libssm.Reader

	once   sync.Once
	secret string
	err    error
}

// Secret は SSM Parameter Store から署名鍵を返す。
func (p *SSMSecretProvider) Secret(ctx context.Context) (string, error) {
	p.once.Do(func() {
		if p.Parameter == nil {
			p.err = fmt.Errorf("SSM parameter is not configured")
			return
		}
		p.secret, p.err = p.Parameter.Get(ctx)
	})
	return p.secret, p.err
}
