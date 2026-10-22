# Flowzen Frontend (HTML, CSS, JavaScript)

No build step. Serve this folder with any static server, e.g.
`npx serve -l 5500 .`, `python -m http.server 5500`, or VS Code Live Server — then open http://localhost:5500.

The backend address is set at the top of `js/app.js` (`http://localhost:8080` by default).
Make sure the backend's `FRONTEND_URL` matches the address you serve this folder from.

## Pages
- index.html – login · register.html – sign up and choose an administrator · verify.html · forgot.html · reset.html
- dashboard.html – role-based dashboard
- requests.html – choose an administrator / approve or reject requests (admins)
- team.html – admin's team, roles, activation, workload
- workspaces.html, workspace.html – workspaces, members, roles
- projects.html, project.html – projects, members, tasks, progress, activity
- tasks.html, task.html – tasks, filters, subtasks, dependencies, comments, mentions, files, history
- notifications.html – notification center
- profile.html – profile, picture, password
- reports.html – reports with date filters
- admin.html – system administration (users, workspaces, projects, activity, system events)

Shared code lives in `js/app.js` and `css/style.css`.
