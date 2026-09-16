package common

import (
	"os"
	"slices"
	"testing"
)

// FunctionNames と backend/cmd/ のディレクトリが一致していることを確認する。
// ずれると ECR / SSM が無い関数を push しようとしたり、Lambda の無いリポジトリができたりする。
func TestFunctionNamesMatchBackendCmd(t *testing.T) {
	entries, err := os.ReadDir("../../backend/cmd")
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	slices.Sort(dirs)

	names := slices.Clone(FunctionNames)
	slices.Sort(names)

	if !slices.Equal(dirs, names) {
		t.Errorf("common.FunctionNames = %v, backend/cmd has %v", names, dirs)
	}
}
