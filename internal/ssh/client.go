package ssh

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// Client handles SSH connections to WP Engine.
type Client struct {
	keyPath    string
	passphrase string
}

// NewClient creates a new SSH client configured with the key path and passphrase.
func NewClient(keyPath, passphrase string) *Client {
	return &Client{
		keyPath:    keyPath,
		passphrase: passphrase,
	}
}

// getAuthMethods retrieves available authentication methods (SSH Agent, then File Key).
func (c *Client) getAuthMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// 1. Try SSH Agent
	if socket := os.Getenv("SSH_AUTH_SOCK"); socket != "" {
		conn, err := net.Dial("unix", socket)
		if err == nil {
			agentClient := agent.NewClient(conn)
			signers, err := agentClient.Signers()
			if err == nil && len(signers) > 0 {
				methods = append(methods, ssh.PublicKeys(signers...))
			}
		}
	}

	// 2. Try File Key
	if c.keyPath != "" {
		// Expand homedir if necessary
		keyPath := c.keyPath
		if strings.HasPrefix(keyPath, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				keyPath = filepath.Join(home, keyPath[2:])
			}
		}

		keyBytes, err := os.ReadFile(keyPath)
		if err == nil {
			var signer ssh.Signer
			if c.passphrase != "" {
				signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(c.passphrase))
			} else {
				signer, err = ssh.ParsePrivateKey(keyBytes)
			}

			if err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no valid SSH authentication methods found (SSH agent is empty and private key could not be read/parsed)")
	}

	return methods, nil
}

// RunWPCLI executes a WP-CLI command on the specified WP Engine environment.
// It returns (stdout, stderr, error).
func (c *Client) RunWPCLI(envName string, wpArgs ...string) (string, string, error) {
	authMethods, err := c.getAuthMethods()
	if err != nil {
		return "", "", fmt.Errorf("ssh auth error: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            envName,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // WP Engine keys can rotate, ignoring is standard for this CLI
		Timeout:         10 * time.Second,
	}

	host := fmt.Sprintf("%s.ssh.wpengine.net:22", envName)
	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return "", "", fmt.Errorf("failed to dial SSH gateway %s: %w", host, err)
	}
	defer conn.Close()

	session, err := conn.NewSession()
	if err != nil {
		return "", "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	// Escape args to safely pass to bash -c
	escapedArgs := make([]string, len(wpArgs))
	for i, arg := range wpArgs {
		// Escape single quotes for bash
		escapedArgs[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
	}

	// Change directory to sites/<envName> and execute wp-cli
	cmd := fmt.Sprintf("cd sites/%s && wp %s", envName, strings.Join(escapedArgs, " "))
	err = session.Run(cmd)

	stdout := strings.TrimSpace(stdoutBuf.String())
	stderr := strings.TrimSpace(stderrBuf.String())

	if err != nil {
		return stdout, stderr, fmt.Errorf("command failed: %w", err)
	}

	return stdout, stderr, nil
}

// VerifyConnection attempts to connect to the environment's SSH server.
func (c *Client) VerifyConnection(envName string) error {
	authMethods, err := c.getAuthMethods()
	if err != nil {
		return err
	}

	config := &ssh.ClientConfig{
		User:            envName,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	host := fmt.Sprintf("%s.ssh.wpengine.net:22", envName)
	conn, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
