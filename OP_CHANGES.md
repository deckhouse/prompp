# OP Changes

## Dev Env

Quick start:

1. Execute:

```shell
make op-develop -f op.mk
```

2. Open http://localhost:3000 Grafana page in your browser and login as `admin:admin` to see the logs

Running services:

| Service    | Description                | Local endpoint                  | Auth           |
|------------|----------------------------|---------------------------------|----------------|
| Grafana    | Logs/metrics visualisation | http://localhost:3000 (Web UI)  | `admin:admin`  |
| Prometheus | Metrics scrape             | http://localhost:9090 (Metrics) |                |

## Updating Prometheus from upstream

1. Lookup required prometheus release at: https://github.com/prometheus/prometheus/releases
2. Clone required prometheus release into custom directory `prometheus-upstream`
```shell
git clone --depth=1 --branch $releaseTag https://github.com/prometheus/prometheus.git prometheus-upstream
```
3. Remove .git and third-party ci related files from `prometheus-upstream`
```shell
cd prometheus-upstream
rm -rf .git
rm -f .gitattributes
rm -rf .github
rm -rf .circleci
```
4. Remove following lines if exists from `.gitignore`:
```
/*.yaml
/*.yml
/vendor
```
5. Checkout <this> prometheus branch into two custom directories `prometheus-update-branch-for-push` and `prometheus-update-branch-for-diff`
6. Remove all codebase-related files and directories from directory `prometheus-update-branch-for-push` (everything except `.helm`, `scripts`, ci and `op`-prefixed files and directories from root)
7. Copy all `prometheus-upstream` codebase-related files to `prometheus-update-branch-for-push`
```shell
cp -a ~/Documents/prometheus-upstream/. ~/Documents/prometheus-update-branch-for-push/
```
8. List files that were previously affected inside `prometheus-update-branch-for-diff` and apply corresponding changes inside: `prometheus-update-branch-for-push`
```shell
make op-list-changes -f op.mk
```
```
./notifier/notifier.go:171: // OP_CHANGES.md: add successfully send metric
./notifier/notifier.go:544: // OP_CHANGES.md: add successfully send metric
./notifier/notifier.go:709: // OP_CHANGES.md: add successfully send metric
...
```
9. Launch Dev Env to check nothing is broken at the moment 
10. Push applied changes to `prometheus-update-branch-for-push` and create the merge request:
```shell
git add -f .
git commit -m "update version"
git push
```