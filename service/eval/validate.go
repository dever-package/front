package eval

import "context"

func Validate(ctx context.Context, req ValidateRequest) (ValidateResult, error) {
	result, err := Run(ctx, req.Request)
	if err != nil {
		return ValidateResult{}, err
	}
	if !req.CompareExpected {
		return ValidateResult{Result: result, Matched: true}, nil
	}

	matched, err := jsonEqual(result.Value, req.Expected)
	if err != nil {
		return ValidateResult{}, Error{Code: ErrorCodeInvalidRequest, Message: "eval expected 必须是 JSON 可序列化数据"}
	}
	if !matched {
		return ValidateResult{}, Error{Code: ErrorCodeExpectedNotMatched, Message: "eval 输出与期望结果不一致"}
	}
	return ValidateResult{Result: result, Matched: true}, nil
}
