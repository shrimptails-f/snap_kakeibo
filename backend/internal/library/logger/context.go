package logger

import "context"

type contextKey struct{}

// ContextWith はフィールドを積んだ新しい ctx を返す。
// 同じキーが既にあれば置き換える。積んだフィールドはその ctx で出力する全ログに付与される。
func ContextWith(ctx context.Context, fields ...Field) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(fields) == 0 {
		return ctx
	}

	current := FieldsFromContext(ctx)
	merged := mergeFields(make([]Field, 0, len(current)+len(fields)), current)
	merged = mergeFields(merged, fields)

	return context.WithValue(ctx, contextKey{}, merged)
}

// FieldsFromContext は ctx に積まれたフィールドを返す。なければ nil。
// 返り値を書き換えても ctx には影響しない。
func FieldsFromContext(ctx context.Context) []Field {
	if ctx == nil {
		return nil
	}
	fields, ok := ctx.Value(contextKey{}).([]Field)
	if !ok || len(fields) == 0 {
		return nil
	}
	return append([]Field(nil), fields...)
}
