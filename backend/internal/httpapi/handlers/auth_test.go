package handlers

import (
	"errors"
	"fmt"
	"testing"

	"github.com/selim/fatura-backend/internal/auth"
)

func TestInvalidTokenMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "wraps reason",
			err:  fmt.Errorf("%w: audience %q is not allowed", auth.ErrInvalidToken, "abc.apps.googleusercontent.com"),
			want: `audience "abc.apps.googleusercontent.com" is not allowed`,
		},
		{
			name: "bare sentinel",
			err:  auth.ErrInvalidToken,
			want: "provider token could not be verified",
		},
		{
			name: "other error is passed through",
			err:  errors.New("boom"),
			want: "boom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := invalidTokenMessage(tc.err); got != tc.want {
				t.Fatalf("invalidTokenMessage() = %q, want %q", got, tc.want)
			}
		})
	}
}
