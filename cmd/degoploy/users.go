package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

	"github.com/degovan/degoploy/internal/admin"
	"github.com/degovan/degoploy/internal/auth"
)

const userUsage = "usage: degoploy user list|create|passwd|role|disable|enable [flags]"

func runUser(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		if !interactive() {
			return errors.New(userUsage)
		}
		sub := "list"
		err := ask(huh.NewSelect[string]().Title("What do you want to do?").Options(
			huh.NewOption("List users", "list"),
			huh.NewOption("Create a user", "create"),
			huh.NewOption("Set a password", "passwd"),
			huh.NewOption("Change a role", "role"),
			huh.NewOption("Disable a user", "disable"),
			huh.NewOption("Enable a user", "enable"),
		).Value(&sub))
		if err != nil {
			return err
		}
		args = append([]string{sub}, args...)
	}
	sub, args := args[0], args[1:]
	fs := flag.NewFlagSet("user "+sub, flag.ExitOnError)
	t := targetFlags(fs)
	username := fs.String("username", "", "username")
	var role, email *string
	var requireChange *bool
	switch sub {
	case "create":
		role = fs.String("role", "", "admin, deployer or viewer (default admin)")
		email = fs.String("email", "", "email address")
	case "role":
		role = fs.String("role", "", "admin, deployer or viewer")
	case "passwd":
		requireChange = fs.Bool("require-change", false, "ask for a new password at the next sign-in")
	case "list", "disable", "enable":
	default:
		return fmt.Errorf("unknown user command %q\n%s", sub, userUsage)
	}
	fs.Parse(args)
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })

	ctx := context.Background()
	c, err := connect(ctx, t)
	if err != nil {
		return err
	}
	defer c.Close()

	switch sub {
	case "list":
		return listUsers(ctx, c)
	case "create":
		return createUser(ctx, c, admin.CreateUserRequest{Username: *username, Email: *email, Role: *role})
	case "passwd":
		return setPassword(ctx, c, *username, *requireChange, given["require-change"])
	case "role":
		return changeRole(ctx, c, *username, *role)
	default:
		return setDisabled(ctx, c, *username, sub == "disable")
	}
}

func listUsers(ctx context.Context, c *admin.Client) error {
	users, err := c.Users(ctx)
	if err != nil {
		return err
	}
	if len(users) == 0 {
		lipgloss.Println(muted.Render("No users yet. Create the first admin with: degoploy user create"))
		return nil
	}
	rows := make([][]string, 0, len(users))
	for _, u := range users {
		status := green.Render("active")
		switch {
		case u.Disabled:
			status = red.Render("disabled")
		case u.MustChangePassword:
			status = yellow.Render("must change password")
		}
		last := muted.Render("never")
		if u.LastLoginAt != nil {
			last = ago(*u.LastLoginAt)
		}
		rows = append(rows, []string{u.Username, u.Role, signInLabel(u.Provider), status, last})
	}
	printTable([]string{"USERNAME", "ROLE", "SIGN-IN", "STATUS", "LAST SIGN-IN"}, rows)
	return nil
}

func createUser(ctx context.Context, c *admin.Client, req admin.CreateUserRequest) error {
	if !interactive() {
		if req.Username == "" {
			return withHint("--username is required", "Run it in a terminal to be asked instead.")
		}
		if req.Role == "" {
			req.Role = "admin"
		}
		pw, err := readPasswordLine()
		if err != nil {
			return err
		}
		req.Password = pw
	} else {
		var form []huh.Field
		if req.Username == "" {
			form = append(form,
				huh.NewInput().Title("Username").Description("Used to sign in.").Value(&req.Username).Validate(validUsername),
				huh.NewInput().Title("Email").Description("Optional.").Value(&req.Email).Validate(validEmail),
			)
		}
		if req.Role == "" {
			req.Role = "deployer"
			if st, err := c.Status(ctx); err == nil && st.Admins == 0 {
				req.Role = "admin"
			}
			form = append(form, roleSelect(&req.Role))
		}
		if len(form) > 0 {
			if err := ask(form...); err != nil {
				return err
			}
		}
		pw, err := newPassword("Password for " + req.Username)
		if err != nil {
			return err
		}
		req.Password = pw
	}
	u, err := c.CreateUser(ctx, req)
	if err != nil {
		return err
	}
	success("Created %s as %s", bold.Render(u.Username), u.Role)
	return nil
}

func setPassword(ctx context.Context, c *admin.Client, username string, requireChange, requireGiven bool) error {
	if username == "" {
		if !interactive() {
			return withHint("--username is required", "Run it in a terminal to be asked instead.")
		}
		var err error
		username, err = pickUser(ctx, c, "Whose password?", func(u admin.User) bool { return u.Provider == "local" })
		if err != nil {
			return err
		}
	}
	var pw string
	var err error
	if interactive() {
		if pw, err = newPassword("New password for " + username); err != nil {
			return err
		}
		if !requireGiven {
			requireChange, err = confirm("Ask for a new password at the next sign-in?",
				"Choose yes when you pass this password on to someone else.", false)
			if err != nil {
				return err
			}
		}
	} else if pw, err = readPasswordLine(); err != nil {
		return err
	}
	if err := c.SetPassword(ctx, username, admin.SetPasswordRequest{Password: pw, RequireChange: requireChange}); err != nil {
		return err
	}
	success("Password updated for %s; their sessions were signed out", bold.Render(username))
	if requireChange {
		lipgloss.Println("  " + muted.Render("They choose a new password when they next sign in."))
	}
	return nil
}

func changeRole(ctx context.Context, c *admin.Client, username, role string) error {
	if (username == "" || role == "") && !interactive() {
		return withHint("--username and --role are required", "Run it in a terminal to be asked instead.")
	}
	if username == "" {
		var err error
		if username, err = pickUser(ctx, c, "Whose role?", nil); err != nil {
			return err
		}
	}
	if role == "" {
		users, err := c.Users(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			if u.Username == username {
				role = u.Role
			}
		}
		if err := ask(roleSelect(&role).Title("New role for " + username)); err != nil {
			return err
		}
	}
	u, err := c.UpdateUser(ctx, username, admin.UpdateUserRequest{Role: &role})
	if err != nil {
		return err
	}
	success("%s is now %s", bold.Render(u.Username), u.Role)
	return nil
}

func setDisabled(ctx context.Context, c *admin.Client, username string, disable bool) error {
	verb := map[bool]string{true: "disable", false: "enable"}[disable]
	if username == "" {
		if !interactive() {
			return withHint("--username is required", "Run it in a terminal to be asked instead.")
		}
		var err error
		username, err = pickUser(ctx, c, "Who do you want to "+verb+"?", func(u admin.User) bool { return u.Disabled != disable })
		if err != nil {
			return err
		}
	}
	if disable && interactive() {
		ok, err := confirm("Disable "+username+"?", "They are signed out everywhere and cannot sign in until enabled again.", false)
		if err != nil {
			return err
		}
		if !ok {
			return errCanceled
		}
	}
	u, err := c.UpdateUser(ctx, username, admin.UpdateUserRequest{Disabled: &disable})
	if err != nil {
		return err
	}
	if u.Disabled {
		success("Disabled %s and signed them out", bold.Render(u.Username))
	} else {
		success("Enabled %s", bold.Render(u.Username))
	}
	return nil
}

// pickUser asks for one of the users that keep returns true for (nil: all).
func pickUser(ctx context.Context, c *admin.Client, title string, keep func(admin.User) bool) (string, error) {
	users, err := c.Users(ctx)
	if err != nil {
		return "", err
	}
	var opts []huh.Option[string]
	for _, u := range users {
		if keep != nil && !keep(u) {
			continue
		}
		label := u.Username + muted.Render("  "+u.Role+" · "+signInLabel(u.Provider))
		if u.Disabled {
			label += muted.Render(" · disabled")
		}
		opts = append(opts, huh.NewOption(label, u.Username))
	}
	if len(opts) == 0 {
		return "", errors.New("no user fits")
	}
	var name string
	return name, ask(huh.NewSelect[string]().Title(title).Options(opts...).Value(&name))
}

func roleSelect(v *string) *huh.Select[string] {
	return huh.NewSelect[string]().Title("Role").Options(
		huh.NewOption("Admin      "+muted.Render("everything, including users and settings"), "admin"),
		huh.NewOption("Deployer   "+muted.Render("deploys, cancels and redeploys"), "deployer"),
		huh.NewOption("Viewer     "+muted.Render("sees projects and logs"), "viewer"),
	).Value(v)
}

// newPassword asks for a password twice until both match.
func newPassword(title string) (string, error) {
	for {
		var pw, again string
		err := ask(
			huh.NewInput().Title(title).
				Description(fmt.Sprintf("At least %d characters.", auth.MinPasswordLen)).
				EchoMode(huh.EchoModePassword).Value(&pw).
				Validate(func(s string) error {
					if len(s) < auth.MinPasswordLen {
						return fmt.Errorf("use at least %d characters", auth.MinPasswordLen)
					}
					return nil
				}),
			huh.NewInput().Title("Repeat it").EchoMode(huh.EchoModePassword).Value(&again),
		)
		if err != nil {
			return "", err
		}
		if pw == again {
			return pw, nil
		}
		warning("The passwords do not match; try again.")
	}
}

// readPasswordLine reads a password from piped stdin, for scripts.
func readPasswordLine() (string, error) {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func validUsername(s string) error {
	if !auth.ValidUsername(strings.TrimSpace(s)) {
		return errors.New(auth.UsernameRule)
	}
	return nil
}

func validEmail(s string) error {
	if s = strings.TrimSpace(s); s != "" && !strings.Contains(s, "@") {
		return errors.New("not a valid email address")
	}
	return nil
}

func signInLabel(provider string) string {
	if provider == "local" {
		return "password"
	}
	return "SSO"
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
	return t.Format("2 Jan 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
