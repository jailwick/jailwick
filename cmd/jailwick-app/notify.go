package main

import (
	"context"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"github.com/jailwick/jailwick/internal/notify"
	"github.com/jailwick/jailwick/internal/server"
)

// appNotifier sends notifications as the app's own: they carry its name and icon, and a click
// opens its page about them. (notify.Send goes through osascript, so macOS shows them as Script
// Editor's, and a click opens Script Editor with its Open dialog.) Where the system has no
// notifications for the app (it isn't a signed bundle, e.g. a dev build) they go out through
// notify.Send; where the user turned them off for jailwick, none are sent.
type appNotifier struct {
	svc   *notifications.NotificationService
	open  func(route string) // shows the window at a route
	ok    atomic.Bool        // the system's notifications started
	asked atomic.Bool        // the user was asked to allow them
}

func newAppNotifier(open func(string)) *appNotifier {
	return &appNotifier{svc: notifications.New(), open: open}
}

func (n *appNotifier) ServiceName() string { return "jailwick/notifications" }

// ServiceStartup starts the system's notifications; failing that is no reason not to start the
// app (a failed service stops it), so it only falls back.
func (n *appNotifier) ServiceStartup(ctx context.Context, opts application.ServiceOptions) error {
	if err := n.svc.ServiceStartup(ctx, opts); err != nil {
		log.Printf("notifications: %v; sending them through the system's tool instead", err)
		return nil
	}
	n.svc.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			return
		}
		route, _ := r.Response.UserInfo["route"].(string)
		n.open(route)
	})
	n.ok.Store(true)
	return nil
}

func (n *appNotifier) ServiceShutdown() error {
	if n.ok.Load() {
		return n.svc.ServiceShutdown()
	}
	return nil
}

// Send shows a notification (server.Server.Notify). The first asks for permission; after that
// the system's settings for jailwick decide.
func (n *appNotifier) Send(x server.Notification) error {
	if !n.ok.Load() {
		return notify.Send(x.Title, x.Body)
	}
	allowed, err := n.svc.CheckNotificationAuthorization()
	if err == nil && !allowed && n.asked.CompareAndSwap(false, true) {
		allowed, err = n.svc.RequestNotificationAuthorization()
	}
	if err != nil || !allowed {
		return err // nil: turned off for jailwick
	}
	return n.svc.SendNotification(notifications.NotificationOptions{
		ID:    fmt.Sprintf("jailwick-%d", time.Now().UnixNano()),
		Title: x.Title,
		Body:  x.Body,
		Data:  map[string]any{"route": x.Route},
	})
}
