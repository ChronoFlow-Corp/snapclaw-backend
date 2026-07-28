package docker

import "github.com/docker/docker/api/types/container"

const GogBootstrapCredentialsPath = "/root/.config/gogcli/bootstrap/credentials.json"

type ExecGmailOptions struct {
	KeyringBackend  string
	KeyringPassword string
}

type ExecPairingApproveOptions struct {
	ChannelType string
	Code        string
}

type ExecGmailWatchStartOptions struct {
	Account         string
	Topic           string
	Labels          []string
	KeyringBackend  string
	KeyringPassword string
}

type ExecGmailWatcherOptions struct {
	Account         string
	WatchPort       string
	WatchPath       string
	HookURL         string
	KeyringBackend  string
	KeyringPassword string
}

func execConnectGmail(opts ExecGmailOptions) container.ExecOptions {
	return container.ExecOptions{
		Cmd: []string{
			"sh",
			"-c",
			`set -eu
mkdir -p /root/.config/gogcli/bootstrap
if [ -f ` + GogBootstrapCredentialsPath + ` ]; then
  gog auth credentials ` + GogBootstrapCredentialsPath + ` >/dev/null 2>&1 || true
fi
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
gog auth tokens import "$tmp"`,
		},
		Env: []string{
			"HOME=/root",
			"XDG_CONFIG_HOME=/root/.config",
			"GOG_KEYRING_BACKEND=" + opts.KeyringBackend,
			"GOG_KEYRING_PASSWORD=" + opts.KeyringPassword,
		},
		User:         "root",
		AttachStdout: true,
		AttachStderr: true,
		AttachStdin:  true,
	}
}

func execPairingApprove(opts ExecPairingApproveOptions) container.ExecOptions {
	return container.ExecOptions{
		Cmd: []string{
			"openclaw",
			"pairing",
			"approve",
			opts.ChannelType,
			opts.Code,
		},
		User:         "node",
		AttachStdout: true,
		AttachStderr: true,
	}
}

func execStartGmailWatch(opts ExecGmailWatchStartOptions) container.ExecOptions {
	cmd := []string{
		"gog", "gmail", "watch", "start",
		"--account", opts.Account,
		"--topic", opts.Topic,
	}
	for _, label := range opts.Labels {
		cmd = append(cmd, "--label", label)
	}

	return container.ExecOptions{
		Cmd: cmd,
		Env: []string{
			"HOME=/root",
			"XDG_CONFIG_HOME=/root/.config",
			"GOG_KEYRING_BACKEND=" + opts.KeyringBackend,
			"GOG_KEYRING_PASSWORD=" + opts.KeyringPassword,
		},
		User:         "root",
		AttachStdout: true,
		AttachStderr: true,
	}
}

func execStartGmailWatcher(opts ExecGmailWatcherOptions) container.ExecOptions {
	return container.ExecOptions{
		Cmd: []string{
			"sh",
			"-c",
			`set -eu
if command -v pkill >/dev/null 2>&1; then
  pkill -f 'gog gmail watch serve' >/dev/null 2>&1 || true
fi

if [ -n "${OPENCLAW_HOOK_TOKEN:-}" ]; then
  nohup gog gmail watch serve \
    --account "$GOG_WATCH_ACCOUNT" \
    --bind 0.0.0.0 \
    --port "$GOG_WATCH_PORT" \
    --path "$GOG_WATCH_PATH" \
    --hook-url "$GOG_WATCH_HOOK_URL" \
    --hook-token "$OPENCLAW_HOOK_TOKEN" \
    >/tmp/gog-watch.log 2>&1 &
else
  nohup gog gmail watch serve \
    --account "$GOG_WATCH_ACCOUNT" \
    --bind 0.0.0.0 \
    --port "$GOG_WATCH_PORT" \
    --path "$GOG_WATCH_PATH" \
    --hook-url "$GOG_WATCH_HOOK_URL" \
    >/tmp/gog-watch.log 2>&1 &
fi`,
		},
		Env: []string{
			"HOME=/root",
			"XDG_CONFIG_HOME=/root/.config",
			"GOG_KEYRING_BACKEND=" + opts.KeyringBackend,
			"GOG_KEYRING_PASSWORD=" + opts.KeyringPassword,
			"GOG_WATCH_ACCOUNT=" + opts.Account,
			"GOG_WATCH_PORT=" + opts.WatchPort,
			"GOG_WATCH_PATH=" + opts.WatchPath,
			"GOG_WATCH_HOOK_URL=" + opts.HookURL,
		},
		User:         "root",
		AttachStdout: true,
		AttachStderr: true,
	}
}
