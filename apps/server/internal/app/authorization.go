package app

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/andreasmolnardev/slopstack/go/authorization"
)

// Authorize is the single backend enforcement point. Direct denies always win;
// group and system-role resolution can be added without changing route callers.
func Authorize(event *core.RequestEvent, permission string) error {
	user, err := CurrentUser(event)
	if err != nil {
		return err
	}
	roles, rules, err := authorization.LoadPocketBase(event.App, user.Id)
	if err != nil {
		return event.InternalServerError("could not load permissions", err)
	}
	principal := authorization.Principal{ID: user.Id, Roles: roles, Superuser: event.HasSuperuserAuth()}
	if authorization.Allows(principal, permission, rules) {
		return nil
	}
	return event.ForbiddenError("missing permission: "+permission, nil)
}
