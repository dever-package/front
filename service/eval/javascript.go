package eval

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
)

func runJavaScript(ctx context.Context, req Request) (Result, error) {
	startedAt := time.Now()
	runtime := goja.New()
	installConsole(runtime)

	input, err := cloneJSONValue(req.Input)
	if err != nil {
		return Result{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval input 必须是 JSON 可序列化数据"}
	}
	config, err := cloneJSONValue(req.Config)
	if err != nil {
		return Result{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval config 必须是 JSON 可序列化数据"}
	}

	if err := runtime.Set("input", input); err != nil {
		return Result{}, err
	}
	if err := runtime.Set("config", config); err != nil {
		return Result{}, err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	done := make(chan struct{})
	defer close(done)
	go interruptRuntime(runtime, timeoutCtx, done)

	program, err := goja.Compile("eval.js", req.Script, true)
	if err != nil {
		return Result{}, Error{Code: ErrorCodeCompileFailed, Message: err.Error()}
	}
	if _, err := runtime.RunProgram(program); err != nil {
		return Result{}, javascriptError(timeoutCtx, err)
	}

	entry := runtime.Get(req.Entry)
	fn, ok := goja.AssertFunction(entry)
	if !ok {
		return Result{}, Error{
			Code:    ErrorCodeEntryMissing,
			Message: fmt.Sprintf("eval script 必须定义函数 %s(input, config)", req.Entry),
		}
	}

	value, err := fn(goja.Undefined(), runtime.ToValue(input), runtime.ToValue(config))
	if err != nil {
		return Result{}, javascriptError(timeoutCtx, err)
	}
	output, err := cloneJSONValue(value.Export())
	if err != nil {
		return Result{}, Error{Code: ErrorCodeInvalidOutput, Message: "eval 输出必须是 JSON 可序列化数据"}
	}

	return Result{
		Language:   req.Language,
		Entry:      req.Entry,
		Value:      output,
		DurationMS: time.Since(startedAt).Milliseconds(),
	}, nil
}

func interruptRuntime(runtime *goja.Runtime, ctx context.Context, done <-chan struct{}) {
	select {
	case <-ctx.Done():
		runtime.Interrupt("eval script 执行超时")
	case <-done:
	}
}

func javascriptError(ctx context.Context, err error) error {
	if ctx.Err() == context.DeadlineExceeded || strings.Contains(err.Error(), "eval script 执行超时") {
		return Error{Code: ErrorCodeTimeout, Message: "eval script 执行超时"}
	}
	return Error{Code: ErrorCodeExecutionFailed, Message: err.Error()}
}

func installConsole(runtime *goja.Runtime) {
	console := runtime.NewObject()
	noop := func(goja.FunctionCall) goja.Value {
		return goja.Undefined()
	}
	_ = console.Set("log", noop)
	_ = console.Set("info", noop)
	_ = console.Set("warn", noop)
	_ = console.Set("error", noop)
	_ = runtime.Set("console", console)
}
