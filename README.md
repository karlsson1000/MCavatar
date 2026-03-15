# MCavatar

Fast Minecraft avatar API. `https://avatar.mcindex.net`

```
/avatar/{user}             face
/avatar/{user}/{size}      face at size (default 128, max 512)
/body/{user}/{size}        full body
/bust/{user}/{size}        head + torso
/skin/{user}               raw 64×64 skin PNG
/uuid/{username}           username + UUID as JSON
```

`user` can be a username or UUID. `size` is in pixels.

## Self-hosting

```bash
docker build -t mcavatar .
docker run -p 8080:8080 -v ./data:/data mcavatar
```

| Variable | Default |
|---|---|
| `PORT` | `8080` |
| `STATS_DB` | `/data/stats.db` |
