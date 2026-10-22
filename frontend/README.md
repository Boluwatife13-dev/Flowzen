# Flowzen frontend (HTML, CSS, JavaScript)

No build step. The Go backend serves this folder, so open http://localhost:8080 when the backend is running.

To host it separately (e.g. Vercel, root directory `frontend`), change `API_URL` at the top of `js/app.js` to your backend link.

- `js/app.js`: shared code (API calls, page layout, forms, helpers)
- `css/style.css`: all styles
- one HTML file per page: login, register, verify, forgot/reset password, dashboard, requests, team,
  workspaces, workspace, projects, project, tasks, task, notifications, profile, reports, admin
