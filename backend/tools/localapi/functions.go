package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/rpc"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/lambda/messages"
)

// modulePath は cmd/<name> の import パスの前半。どのディレクトリから起動しても go build できるようにする。
const modulePath = "snap_kakeibo/backend/cmd/"

// readyTimeout は子プロセスが RPC を受け付けるまで待つ上限。init() で Floci の SSM を読むぶんを見込む。
const readyTimeout = 30 * time.Second

// function はローカル RPC モードで起動した Lambda 1 本。
type function struct {
	name   string
	cmd    *exec.Cmd
	client *rpc.Client
	// exited は子プロセスが終了したら閉じる
	exited chan struct{}
}

// functionPool は起動済みの Lambda 群。
type functionPool struct {
	functions map[string]*function
	binDir    string
	timeout   time.Duration
}

// startFunctions は names の Lambda をまとめてビルドし、それぞれ子プロセスとして起動して RPC が通るまで待つ。
func startFunctions(ctx context.Context, names []string, environment map[string]string, timeout time.Duration) (*functionPool, error) {
	binDir, err := os.MkdirTemp("", "snap-kakeibo-localapi-")
	if err != nil {
		return nil, err
	}
	pool := &functionPool{functions: map[string]*function{}, binDir: binDir, timeout: timeout}
	if err := buildFunctions(ctx, binDir, names); err != nil {
		pool.Close()
		return nil, err
	}
	for _, name := range names {
		fn, err := startFunction(ctx, filepath.Join(binDir, name), name, environment)
		if err != nil {
			pool.Close()
			return nil, err
		}
		pool.functions[name] = fn
	}
	return pool, nil
}

// buildFunctions は -o にディレクトリを渡して複数 main package を 1 回の go build で作る。
func buildFunctions(ctx context.Context, binDir string, names []string) error {
	args := []string{"build", "-o", binDir + string(filepath.Separator)}
	for _, name := range names {
		args = append(args, modulePath+name)
	}
	log.Printf("building %d functions", len(names))
	started := time.Now()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	log.Printf("built in %s", time.Since(started).Round(time.Millisecond))
	return nil
}

func startFunction(ctx context.Context, binary, name string, environment map[string]string) (*function, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(),
		"_LAMBDA_SERVER_PORT="+strconv.Itoa(port),
		"AWS_LAMBDA_FUNCTION_NAME=local-"+name,
	)
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	// ctx が閉じたら SIGTERM、それでも残れば WaitDelay 後に SIGKILL
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 3 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	fn := &function{name: name, cmd: cmd, exited: make(chan struct{})}
	go prefixLines(name, stdout)
	go prefixLines(name, stderr)
	go func() {
		defer close(fn.exited)
		if err := cmd.Wait(); err != nil && ctx.Err() == nil {
			log.Printf("[%s] exited: %v", name, err)
		}
	}()

	client, err := waitForRPC(port, fn.exited)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	fn.client = client
	log.Printf("[%s] ready on port %d", name, port)
	return fn, nil
}

// waitForRPC は子プロセスが RPC を受け付けるまで接続を試す。先にプロセスが終わったら諦める。
func waitForRPC(port int, exited <-chan struct{}) (*rpc.Client, error) {
	deadline := time.Now().Add(readyTimeout)
	for {
		client, err := rpc.Dial("tcp", net.JoinHostPort("localhost", strconv.Itoa(port)))
		if err == nil {
			if err := client.Call("Function.Ping", &messages.PingRequest{}, &messages.PingResponse{}); err == nil {
				return client, nil
			}
			_ = client.Close()
		}
		select {
		case <-exited:
			return nil, errors.New("process exited before accepting RPC (see its log above)")
		case <-time.After(100 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("not ready after %s", readyTimeout)
		}
	}
}

// invoke は API Gateway と同じく JSON のイベントを渡し、JSON のレスポンスを受け取る。
// ハンドラがエラーを返した(または panic した)場合は messages.InvokeResponse_Error を返す。
func (p *functionPool) invoke(name string, payload []byte) ([]byte, error) {
	fn, ok := p.functions[name]
	if !ok {
		return nil, fmt.Errorf("function %s is not running", name)
	}
	select {
	case <-fn.exited:
		return nil, fmt.Errorf("function %s has exited; restart localapi", name)
	default:
	}
	req := &messages.InvokeRequest{
		Payload:            payload,
		RequestId:          requestID(),
		Deadline:           messages.InvokeRequest_Timestamp{Seconds: time.Now().Add(p.timeout).Unix()},
		InvokedFunctionArn: "arn:aws:lambda:local:000000000000:function:local-" + name,
	}
	var res messages.InvokeResponse
	if err := fn.client.Call("Function.Invoke", req, &res); err != nil {
		return nil, fmt.Errorf("rpc %s: %w", name, err)
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Payload, nil
}

// Close は子プロセスを止め、ビルド成果物を消す。
func (p *functionPool) Close() {
	var wg sync.WaitGroup
	for _, fn := range p.functions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if fn.client != nil {
				_ = fn.client.Close()
			}
			_ = fn.cmd.Process.Signal(os.Interrupt)
			select {
			case <-fn.exited:
			case <-time.After(fn.cmd.WaitDelay):
				_ = fn.cmd.Process.Kill()
				<-fn.exited
			}
		}()
	}
	wg.Wait()
	_ = os.RemoveAll(p.binDir)
}

// freePort は空いている TCP ポートを OS に選ばせる。閉じてから子に渡すので厳密には競合し得るが、ローカル用途では十分。
func freePort() (int, error) {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// prefixLines は子プロセスの出力を [name] 付きでそのまま流す。Lambda の logger は JSON を出すので加工しない。
func prefixLines(name string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fmt.Fprintf(os.Stdout, "[%s] %s\n", name, scanner.Text())
	}
}

func requestID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "local"
	}
	return hex.EncodeToString(b)
}
