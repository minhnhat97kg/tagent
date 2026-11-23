# tagent web UI

Source for the browser UI that `tagent web` serves. `make web` builds it into `internal/web/dist`, which the binary embeds.

Dev: run `tagent web --addr 127.0.0.1:7777 --token t` (the Vite dev server proxies /api there), open `http://127.0.0.1:7777/?token=t` once to set the cookie, then run `npm run dev` in this folder and use `http://127.0.0.1:5173`.
