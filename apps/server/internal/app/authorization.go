package app

import "github.com/pocketbase/pocketbase/core"

// Authorize is the single backend enforcement point. Direct denies always win;
// group and system-role resolution can be added without changing route callers.
func Authorize(event *core.RequestEvent, permission string) error {
	if _, err := CurrentUser(event); err != nil {
		return err
	}
	if event.HasSuperuserAuth() {
		return nil
	}
	return event.ForbiddenError("missing permission: "+permission, nil)
}
