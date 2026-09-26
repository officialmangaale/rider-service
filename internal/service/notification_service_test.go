package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestDeviceRegistrationsThatCannotBeRealTokensAreRefusedBeforeTheDatabase(t *testing.T) {
	// A nil repository proves nothing is stored: a valid call would panic.
	svc := NewNotificationService(nil)
	ctx := context.Background()

	bad := []struct{ rider, platform, token string }{
		{"", "android", "tok"},
		{"r1", "", "tok"},
		{"r1", "windows", "tok"},
		{"r1", "android", ""},
		{"r1", "android", "   "},
		{"r1", "android", "has space"},
		{"r1", "android", "line\nbreak"},
		{"r1", "android", strings.Repeat("x", maxPushTokenLength+1)},
	}
	for _, c := range bad {
		if err := svc.RegisterDeviceToken(ctx, c.rider, c.platform, c.token); !errors.Is(err, ErrInvalidDeviceToken) {
			t.Errorf("%+v: err=%v", c, err)
		}
	}
	if err := svc.UnregisterDeviceToken(ctx, "r1", "  "); !errors.Is(err, ErrInvalidDeviceToken) {
		t.Errorf("unregister empty: %v", err)
	}
	if err := svc.UnregisterDeviceToken(ctx, "", "tok"); !errors.Is(err, ErrInvalidDeviceToken) {
		t.Errorf("unregister without rider: %v", err)
	}
}
