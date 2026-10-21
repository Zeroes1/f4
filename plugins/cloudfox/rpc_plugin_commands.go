package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/sdk/f4plugin"
)

type rpcPrompter interface {
	Prompt(f4plugin.PromptRequest) (f4plugin.PromptResponse, error)
}

func (p *RPCPlugin) ask(label, value string, secret bool) (string, error) {
	if p.prompter == nil {
		return "", errors.New("cloudfox: plugin host is not initialized")
	}
	response, err := p.prompter.Prompt(f4plugin.PromptRequest{Title: "CloudFox", Prompt: label, Default: value, Password: secret})
	if err != nil {
		return "", err
	}
	if !response.Accepted {
		return "", context.Canceled
	}
	return response.Text, nil
}

func (p *RPCPlugin) PluginCommands() []f4plugin.PluginCommand {
	return []f4plugin.PluginCommand{
		{ID: cloudFoxAddConnectionCommandID, Label: "CloudFox: Add connection", LocalizedLabels: map[string]string{"ru": "CloudFox: Добавить подключение"}, ActiveDrives: []string{DriveName}},
		{ID: cloudFoxEditConnectionCommandID, Label: "CloudFox: Edit connection", LocalizedLabels: map[string]string{"ru": "CloudFox: Изменить подключение"}, ActiveDrives: []string{DriveName}},
		{ID: "cloudfox.unlock-vault", Label: "CloudFox: Unlock vault", LocalizedLabels: map[string]string{"ru": "CloudFox: Разблокировать хранилище"}, ActiveDrives: []string{DriveName}},
	}
}

func (p *RPCPlugin) RunPluginCommand(id string) error {
	err := p.runCommand(id)
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func (p *RPCPlugin) runCommand(id string) error {
	ctx := context.Background()
	if id == "cloudfox.unlock-vault" {
		if p.plugin.vault == nil {
			return nil
		}
		return p.plugin.vault.ensureUnlocked(ctx)
	}
	var original *Connection
	switch id {
	case cloudFoxAddConnectionCommandID:
	case cloudFoxEditConnectionCommandID:
		connections, err := p.plugin.repo.List(ctx)
		if err != nil {
			return err
		}
		if len(connections) == 0 {
			return ErrConnectionNotFound
		}
		names := make([]string, len(connections))
		for i, c := range connections {
			names[i] = c.Name
		}
		name, err := p.ask("Connection to edit: "+strings.Join(names, ", "), names[0], false)
		if err != nil {
			return err
		}
		for _, c := range connections {
			if c.Name == name {
				clone := c.Clone()
				original = &clone
				break
			}
		}
		if original == nil {
			return ErrConnectionNotFound
		}
	default:
		return fmt.Errorf("cloudfox: unknown command %q", id)
	}
	return p.editProfile(ctx, original)
}

func (p *RPCPlugin) editProfile(ctx context.Context, original *Connection) error {
	connection := Connection{}
	if original == nil {
		provider, err := p.ask("Storage type (gdrive, yandex, s3, webdav)", "webdav", false)
		if err != nil {
			return err
		}
		connection.Provider = ProviderType(strings.ToLower(strings.TrimSpace(provider)))
	} else {
		connection = original.Clone()
	}
	factory, ok := p.plugin.Factory(connection.Provider)
	if !ok {
		return ErrFactoryNotRegistered
	}
	name, err := p.ask("Connection name", connection.Name, false)
	if err != nil {
		return err
	}
	connection.Name = strings.TrimSpace(name)
	if err := validateConnection(connection); err != nil {
		return err
	}
	settings := map[string]any{}
	if len(connection.Settings) != 0 {
		if err := json.Unmarshal(connection.Settings, &settings); err != nil {
			return err
		}
	}
	var secretKeys []string
	var secretLabels []string
	var promptErr error
	field := func(key, label, initial string) {
		if promptErr != nil {
			return
		}
		if value, ok := settings[key].(string); ok {
			initial = value
		}
		var value string
		value, promptErr = p.ask(label, initial, false)
		settings[key] = strings.TrimSpace(value)
	}
	boolean := func(key, label string) {
		if promptErr != nil {
			return
		}
		initial := "no"
		if settings[key] == true {
			initial = "yes"
		}
		var value string
		value, promptErr = p.ask(label+" (yes/no)", initial, false)
		if promptErr != nil {
			return
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "yes":
			settings[key] = true
		case "no":
			settings[key] = false
		default:
			promptErr = errors.New("cloudfox: answer yes or no")
		}
	}
	switch connection.Provider {
	case ProviderWebDAV:
		field("base_url", "Server URL", "")
		field("root", "Remote root", "/")
		field("auth", "Authentication (basic, digest, bearer, anonymous)", "basic")
		field("username", "Username", "")
		field("custom_ca", "Custom CA file (optional)", "")
		boolean("allow_insecure_digest", "Allow Digest over plain HTTP")
		if settings["auth"] == "bearer" {
			secretKeys, secretLabels = []string{"bearer_token"}, []string{"Bearer token"}
		} else if settings["auth"] != "anonymous" {
			secretKeys, secretLabels = []string{"password"}, []string{"Password"}
		}
	case ProviderS3:
		field("bucket", "Bucket (empty discovers buckets)", "")
		field("region", "Region", "us-east-1")
		field("root_prefix", "Root prefix", "")
		field("endpoint", "Custom endpoint (optional)", "")
		field("auth", "Authentication (default, profile, static, anonymous)", "default")
		field("profile", "AWS profile (optional)", "")
		field("custom_ca", "Custom CA file (optional)", "")
		boolean("use_path_style", "Use path-style addressing")
		boolean("allow_insecure", "Allow credentials over plain HTTP")
		if settings["auth"] == "static" {
			secretKeys, secretLabels = []string{"access_key_id", "secret_access_key", "session_token"}, []string{"Access key ID", "Secret access key", "Session token (optional)"}
		}
	case ProviderYandexDisk:
		field("client_id", "OAuth client ID (optional with token)", DefaultYandexClientID)
		field("root", "Remote root", "/")
		secretKeys, secretLabels = []string{"oauth_token"}, []string{"OAuth token"}
	case ProviderGoogleDrive:
		field("client_id", "OAuth client ID", DefaultGoogleClientID)
		secretKeys, secretLabels = []string{"client_secret", "refresh_token", "access_token"}, []string{"OAuth client secret (optional)", "OAuth refresh token", "OAuth access token (optional)"}
	}
	if promptErr != nil {
		return promptErr
	}
	connection.Settings, err = json.Marshal(settings)
	if err != nil {
		return err
	}
	if err := factory.Validate(connection); err != nil {
		return err
	}
	typed := SecretValues{}
	defer clearSecrets(typed)
	for i, key := range secretKeys {
		value, err := p.ask(secretLabels[i], "", true)
		if err != nil {
			return err
		}
		if value != "" {
			typed[key] = value
		}
	}
	changed, err := validateCredentialScopeChange(original, connection, typed)
	if err != nil {
		return err
	}
	values := typed.Clone()
	if original != nil && original.SecretRef != "" && !changed {
		stored, err := p.plugin.repo.Credentials(ctx, *original)
		if err != nil {
			return err
		}
		values = mergeSecretValues(stored, typed)
		clearSecrets(stored)
	}
	defer clearSecrets(values)
	sanitizeSecretsForConnection(connection, values)
	if err := validateRequiredSecrets(connection, values); err != nil {
		return err
	}
	storage := SecretStorageKeyring
	if p.plugin.portable || (original != nil && strings.HasPrefix(original.SecretRef, "vault:")) {
		storage = SecretStorageVault
	}
	var credentials *SecretValues
	if len(values) != 0 || connection.SecretRef != "" {
		credentials = &values
	}
	if _, err := p.plugin.repo.Save(ctx, connection, credentials, storage); err != nil {
		return err
	}
	if original != nil {
		p.forgetConnection(original.Name)
	}
	return nil
}

type rpcMasterPasswordPrompter struct{ plugin *RPCPlugin }

func (r rpcMasterPasswordPrompter) PromptMasterPassword(ctx context.Context, creating bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	password, err := r.plugin.ask("Master password", "", true)
	if err != nil {
		return "", err
	}
	if creating {
		confirmation, err := r.plugin.ask("Repeat master password", "", true)
		if err != nil {
			return "", err
		}
		if password != confirmation {
			return "", errors.New("cloudfox: passwords do not match")
		}
		if password == "" {
			answer, err := r.plugin.ask(emptyMasterPasswordWarning+" Type YES to continue.", "", false)
			if err != nil {
				return "", err
			}
			if answer != "YES" {
				return "", context.Canceled
			}
		}
	}
	return password, nil
}

var _ f4plugin.CommandProvider = (*RPCPlugin)(nil)
