// Package oswrapper は環境変数とファイルの読み取りを差し替え可能にする薄いラッパーを提供する。
//
// 業務ロジックは os.Getenv / os.ReadFile を直接呼ばず Interface を受け取る。
// GetEnv は未設定と空文字をエラーにするので、必須の設定が抜けていることに起動時に気づける。
//
//	osw := oswrapper.New()
//	table, err := osw.GetEnv("ANALYSIS_REQUESTS_TABLE")
package oswrapper

// Interface は OS に対する読み取り操作の契約。
type Interface interface {
	// ReadFile はファイルを読み込み文字列として返す。
	ReadFile(path string) (string, error)
	// GetEnv は環境変数を返す。未設定または空白のみならエラー。
	GetEnv(key string) (string, error)
}
