# Flowzen Backend (Go + MongoDB)

## Run
1. Install Go 1.22+ and have MongoDB running (local or MongoDB Atlas).
2. `cp .env.example .env` and set `MONGO_URI`, `JWT_SECRET`, `FRONTEND_URL` (and SMTP for real emails).
3. `go mod tidy` (first time only — downloads dependencies)
4. `go run .`  → API on http://localhost:8080

On first start a system administrator is created from `SYSADMIN_EMAIL` / `SYSADMIN_PASSWORD`.
Change that password after your first login.

## Roles
- **sysadmin** – whole platform; the only role that can create administrators (System Admin → Users → role "admin").
- **admin** – leads a team, approves access requests, owns workspaces in their scope.
- **manager** – can create workspaces/projects inside their admin's scope.
- **member** – works on assigned tasks.
New sign-ups are members and must be approved by the administrator they choose before they can see workspaces.

## Email
Leave `SMTP_HOST` empty during development: verification, reset and notification emails are printed to the console
(copy the link from there). For Gmail use `smtp.gmail.com`, port 587 and an App Password. Failed emails are retried once
and recorded under System Admin → System events.

## Files
| File | What it covers |
|---|---|
| main.go | config, MongoDB connection, CORS, helpers |
| auth.go | register, verify email, login (JWT), forgot/reset/change password, logout, profile |
| teams.go | administrators, access requests, team management, system admin users & logs |
| workspaces.go | workspaces, members and roles |
| projects.go | projects, members, progress |
| tasks.go | tasks, subtasks, dependencies, assignment, filtering/sorting |
| collab.go | comments/replies/mentions, attachments, avatars, activity |
| notify.go | in-app notifications, email, activity log, deadline/overdue reminders |
| reports.go | dashboard, reports, all routes |

Uploaded files are stored in `UPLOAD_DIR` (default `uploads/`); keep that folder on persistent storage in production.
