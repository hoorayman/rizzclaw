package multiagent

import (
	"context"
)

type ctxKey int

const (
	channelCtxKey ctxKey = iota
	chatIDCtxKey
)

func ContextWithChannel(ctx context.Context, channel, chatID string) context.Context {
	ctx = context.WithValue(ctx, channelCtxKey, channel)
	ctx = context.WithValue(ctx, chatIDCtxKey, chatID)
	return ctx
}

func GetChannelFromContext(ctx context.Context) string {
	if v := ctx.Value(channelCtxKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func GetChatIDFromContext(ctx context.Context) string {
	if v := ctx.Value(chatIDCtxKey); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
