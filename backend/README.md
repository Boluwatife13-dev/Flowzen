# Flowzen backend (Go + MongoDB)

The backend is the API and also serves the website pages from `../frontend`.

## Run on your computer
1. Install Go 1.22+ and have MongoDB ready (local or Atlas).
2. Copy `.env.example` to `.env` and set `MONGO_URI`, `DB_NAME`, `JWT_SECRET`, `SYSADMIN_EMAIL`, `SYSADMIN_PASSWORD`.
3. `go mod tidy` (first time only)
4. `go run .` and open http://localhost:8080

Without SMTP settings, emails (verification, reset, notifications) are printed in the terminal.

## Deploy on Render
- Root Directory: empty
- Build Command: `cd backend && go build -o app .`
- Start Command: `cd backend && ./app`
- Environment: MONGO_URI, DB_NAME, JWT_SECRET, FRONTEND_URL (your live link), SYSADMIN_EMAIL, SYSADMIN_PASSWORD

## Folder structure
```
backend/
├── main.go                 starts everything
├── config/config.go        settings from .env
├── database/database.go    MongoDB connection and system event log
├── models/                 the data saved in MongoDB
│   ├── user.go  request.go  workspace.go  project.go
│   └── task.go  file.go  comment.go  notification.go  activity.go
├── utils/                  shared helpers
│   ├── errors.go           Handler type, Fail(), Msg()
│   ├── request.go          reading JSON, ids and paging from requests
│   ├── query.go            FindOne, FindAll, FindPage, UsersByID
│   ├── helpers.go          passwords, tokens, dates, search
│   └── upload.go           safe file uploads
├── middleware/
│   ├── auth.go             login tokens and access levels
│   ├── respond.go          turns handler results into JSON
│   └── cors.go
├── services/               business rules used by several handlers
│   ├── permissions.go      workspace and project roles
│   ├── members.go          member list helpers
│   ├── projects.go         visible projects, progress, delete data
│   ├── team.go             access requests, leaving a team
│   ├── notify.go  email.go  activity.go
│   ├── reminders.go        due-soon and overdue alerts every 15 minutes
│   └── setup.go            creates the first system admin
├── handlers/               one function per API endpoint
│   ├── auth.go  profile.go  requests.go  team.go  admin.go
│   ├── workspaces.go  workspace_members.go  projects.go  project_members.go
│   ├── tasks.go  task_access.go  subtasks.go  comments.go  files.go
│   └── notifications.go  dashboard.go  reports.go  health.go
└── routes/routes.go        every URL and who can use it
```

Each handler returns `(data, error)`. Return `utils.Fail(400, "message")` for an error the user should see.

## Roles
- **sysadmin**: the whole platform; the only one who can make someone an admin
- **admin**: leads a team and approves access requests
- **manager**: can create workspaces and projects inside their admin's team
- **member**: works on tasks

Workspace roles: owner, manager, member. Project roles: manager, contributor, viewer.
