package k8sfs

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	errNoKubeconfig = errors.New("k8sfs: no kubeconfig found")
	errUnsupported  = errors.New("k8sfs: unsupported kubeconfig")
)

// kubeconfig is the part of a kubeconfig file that a client needs.
type kubeconfig struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                string `yaml:"server"`
			CertificateAuthority  string `yaml:"certificate-authority"`
			CertificateAuthorityD string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify bool   `yaml:"insecure-skip-tls-verify"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token                 string         `yaml:"token"`
			TokenFile             string         `yaml:"tokenFile"`
			ClientCertificate     string         `yaml:"client-certificate"`
			ClientCertificateData string         `yaml:"client-certificate-data"`
			ClientKey             string         `yaml:"client-key"`
			ClientKeyData         string         `yaml:"client-key-data"`
			Exec                  map[string]any `yaml:"exec"`
			AuthProvider          map[string]any `yaml:"auth-provider"`
		} `yaml:"user"`
	} `yaml:"users"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

// apiEndpoint is what a kubeconfig context resolves to.
type apiEndpoint struct {
	server string // https://host:port
	token  string
	tls    *tls.Config
}

// kubeconfigPath is where the config lives: the first entry of KUBECONFIG, or
// ~/.kube/config.
func kubeconfigPath() (string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		for _, p := range filepath.SplitList(env) {
			if p != "" {
				return p, nil
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errNoKubeconfig
	}
	return filepath.Join(home, ".kube", "config"), nil
}

// loadEndpoint reads the kubeconfig and resolves its current context.
// Credentials that need a helper program (exec plugins, cloud auth providers)
// are refused with a message that names the reason rather than sent without
// credentials.
func loadEndpoint(configPath string) (*apiEndpoint, error) {
	raw, err := os.ReadFile(configPath) // #nosec G304 -- the user's own kubeconfig
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w (%s)", errNoKubeconfig, configPath)
		}
		return nil, err
	}
	return parseEndpoint(raw, filepath.Dir(configPath))
}

func parseEndpoint(raw []byte, baseDir string) (*apiEndpoint, error) {
	var cfg kubeconfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", errUnsupported, err)
	}
	ctxName := cfg.CurrentContext
	if ctxName == "" && len(cfg.Contexts) == 1 {
		ctxName = cfg.Contexts[0].Name
	}
	var clusterName, userName string
	for _, c := range cfg.Contexts {
		if c.Name == ctxName {
			clusterName, userName = c.Context.Cluster, c.Context.User
		}
	}
	if clusterName == "" {
		return nil, fmt.Errorf("%w: no current context", errUnsupported)
	}
	ep := &apiEndpoint{}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	found := false
	for _, c := range cfg.Clusters {
		if c.Name != clusterName {
			continue
		}
		found = true
		u, err := url.Parse(c.Cluster.Server)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("%w: bad server %q", errUnsupported, c.Cluster.Server)
		}
		ep.server = strings.TrimRight(c.Cluster.Server, "/")
		tlsCfg.InsecureSkipVerify = c.Cluster.InsecureSkipTLSVerify // #nosec G402 -- the kubeconfig asked for it
		ca, err := fileOrData(baseDir, c.Cluster.CertificateAuthority, c.Cluster.CertificateAuthorityD)
		if err != nil {
			return nil, err
		}
		if ca != nil {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(ca) {
				return nil, fmt.Errorf("%w: the cluster CA is not PEM", errUnsupported)
			}
			tlsCfg.RootCAs = pool
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: cluster %q is not defined", errUnsupported, clusterName)
	}
	for _, u := range cfg.Users {
		if u.Name != userName {
			continue
		}
		if len(u.User.Exec) > 0 || len(u.User.AuthProvider) > 0 {
			return nil, fmt.Errorf("%w: user %q gets credentials from a helper program, which is not supported yet; use a token or a client certificate", errUnsupported, userName)
		}
		ep.token = u.User.Token
		if ep.token == "" && u.User.TokenFile != "" {
			tok, err := os.ReadFile(resolve(baseDir, u.User.TokenFile)) // #nosec G304 -- named by the user's kubeconfig
			if err != nil {
				return nil, err
			}
			ep.token = strings.TrimSpace(string(tok))
		}
		certPEM, err := fileOrData(baseDir, u.User.ClientCertificate, u.User.ClientCertificateData)
		if err != nil {
			return nil, err
		}
		keyPEM, err := fileOrData(baseDir, u.User.ClientKey, u.User.ClientKeyData)
		if err != nil {
			return nil, err
		}
		if certPEM != nil && keyPEM != nil {
			pair, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("%w: client certificate: %v", errUnsupported, err)
			}
			tlsCfg.Certificates = []tls.Certificate{pair}
		}
	}
	ep.tls = tlsCfg
	return ep, nil
}

func resolve(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

// fileOrData reads a credential given either as a file name or as inline
// base64, and returns nil when neither is set.
func fileOrData(baseDir, file, data string) ([]byte, error) {
	if data != "" {
		out, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("%w: bad base64 credential: %v", errUnsupported, err)
		}
		return out, nil
	}
	if file != "" {
		return os.ReadFile(resolve(baseDir, file)) // #nosec G304 -- named by the user's kubeconfig
	}
	return nil, nil
}
