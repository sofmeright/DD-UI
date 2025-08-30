package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type sshCfg struct {
	User   string
	Port   string
	KeyPem string
	Sudo   bool
	Strict bool
}

func loadSSH() (sshCfg, error) {
	key, err := readSecretMaybeFile(env("DDUI_SSH_KEY", "")) // supports "@/run/secrets/…"
	if err != nil {
		return sshCfg{}, err
	}
	return sshCfg{
		User:   env("DDUI_SSH_USER", "root"),
		Port:   env("DDUI_SSH_PORT", "22"),
		KeyPem: strings.TrimSpace(key),
		Sudo:   strings.ToLower(env("DDUI_SSH_USE_SUDO", "false")) == "true",
		Strict: strings.ToLower(env("DDUI_SSH_STRICT_HOST_KEY", "false")) == "true",
	}, nil
}

func dialSSH(h HostRow) (*ssh.Client, error) {
	cfg, err := loadSSH()
	if err != nil {
		return nil, err
	}
	signer, err := ssh.ParsePrivateKey([]byte(cfg.KeyPem))
	if err != nil {
		return nil, fmt.Errorf("ssh key parse: %w", err)
	}
	cb := ssh.InsecureIgnoreHostKey()
	if cfg.Strict {
		// TODO: add known_hosts verification; left off for MVP
	}
	clientCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: cb,
		Timeout:         10 * time.Second,
	}
	addr := fmt.Sprintf("%s:%s", firstNonEmpty(h.Addr, h.Name), cfg.Port)
	return ssh.Dial("tcp", addr, clientCfg)
}

func runSSH(client *ssh.Client, cmd string) (string, string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer sess.Close()
	var out, errb bytes.Buffer
	sess.Stdout = &out
	sess.Stderr = &errb
	err = sess.Run(cmd)
	return out.String(), errb.String(), err
}

func dockerCmd(base string) string {
	if strings.ToLower(env("DDUI_SSH_USE_SUDO", "false")) == "true" {
		return "sudo " + base
	}
	return base
}

// Minimal subset of `docker inspect` we care about
type dockerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"` // starts with "/"
	Config struct {
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	State struct {
		Status  string `json:"Status"`  // "running", "exited"
		Running bool   `json:"Running"` // true/false
	} `json:"State"`
	NetworkSettings struct {
		Ports any `json:"Ports"` // keep raw; JSON friendly
	} `json:"NetworkSettings"`
}

func ScanHostContainers(ctx context.Context, hostName string) (int, error) {
	// lookup host
	row, err := getHostRow(ctx, hostName)
	if err != nil {
		return 0, err
	}
	cli, err := dialSSH(row)
	if err != nil {
		return 0, err
	}
	defer cli.Close()

	// 1) list container IDs
	psCmd := dockerCmd(`docker ps -q`)
	stdout, stderr, err := runSSH(cli, psCmd)
	if err != nil {
		// if docker not installed / permission issue, return verbose error
		return 0, fmt.Errorf("ps: %v; %s", err, strings.TrimSpace(stderr))
	}
	ids := strings.Fields(stdout)
	if len(ids) == 0 {
		// nothing running; you might still want to clear stale rows later if desired
		return 0, nil
	}

	// 2) inspect all in one shot
	inspectCmd := dockerCmd(`docker inspect --format='{{json .}}' ` + strings.Join(ids, " "))
	stdout, stderr, err = runSSH(cli, inspectCmd)
	if err != nil {
		return 0, fmt.Errorf("inspect: %v; %s", err, strings.TrimSpace(stderr))
	}

	// 3) parse and upsert
	hostID, err := hostIDByName(ctx, hostName)
	if err != nil {
		return 0, err
	}
	lines := splitNonEmptyLines(stdout)
	n := 0
	for _, ln := range lines {
		var di dockerInspect
		if err := json.Unmarshal([]byte(ln), &di); err != nil {
			continue // skip malformed
		}
		lbls := map[string]string{}
		for k, v := range di.Config.Labels {
			lbls[k] = v
		}
		cr := ContainerRow{
			HostID:      hostID,
			ContainerID: di.ID,
			Name:        strings.TrimPrefix(di.Name, "/"),
			Image:       di.Config.Image,
			State:       di.State.Status,
			Status:      di.State.Status,
			Labels:      lbls,
			Owner:       row.Owner,
		}
		// raw Ports -> slice
		switch p := di.NetworkSettings.Ports.(type) {
		case map[string]any:
			cr.Ports = []any{p}
		case []any:
			cr.Ports = p
		default:
			cr.Ports = []any{}
		}
		// compose grouping (if present)
		cr.ComposeProj = lbls["com.docker.compose.project"]
		cr.ComposeSvc  = lbls["com.docker.compose.service"]

		if err := upsertContainer(ctx, cr); err == nil {
			n++
		}
	}
	return n, nil
}

func getHostRow(ctx context.Context, name string) (HostRow, error) {
	var r HostRow
	err := db.QueryRow(ctx, `SELECT id, name, addr, "groups", labels, owner FROM hosts WHERE name=$1`, name).
		Scan(&r.ID, &r.Name, &r.Addr, &r.Groups, &r.Labels, &r.Owner)
	return r, err
}

func splitNonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
