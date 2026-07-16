package agentdock

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/uvwt/nexusdock/internal/core"
)

const (
	defaultTimeoutSeconds = 8
	maxTokenBytes         = 16 * 1024
	secretVersion         = byte(1)
)

var (
	ErrNodeNotFound          = errors.New("AgentDock 节点不存在")
	ErrNodeExists            = errors.New("AgentDock 节点 ID 或地址已存在")
	ErrNodeDisabled          = errors.New("AgentDock 节点已停用")
	ErrNodeCredentialsAbsent = errors.New("AgentDock 节点凭据缺失")

	nodeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string { return e.Message }

func invalid(message string) error { return ValidationError{Message: message} }

type Node struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Endpoint        string    `json:"endpoint"`
	Enabled         bool      `json:"enabled"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	TokenConfigured bool      `json:"token_configured"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Credentials struct {
	Node  Node
	Token string
}

type CreateInput struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Endpoint       string `json:"endpoint"`
	Token          string `json:"token"`
	Enabled        *bool  `json:"enabled,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type UpdateInput struct {
	Name           *string `json:"name,omitempty"`
	Endpoint       *string `json:"endpoint,omitempty"`
	Token          *string `json:"token,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	TimeoutSeconds *int    `json:"timeout_seconds,omitempty"`
}

type Store struct {
	db     *sql.DB
	cipher cipher.AEAD
	now    func() time.Time
}

func NewStore(db *sql.DB, dataDir string) (*Store, error) {
	if db == nil {
		return nil, errors.New("AgentDock 节点数据库不能为空")
	}
	key, err := loadOrCreateKey(filepath.Join(dataDir, "secrets", "agentdock-nodes.key"))
	if err != nil {
		return nil, err
	}
	return NewStoreWithKey(db, key)
}

func NewStoreWithKey(db *sql.DB, key []byte) (*Store, error) {
	if db == nil {
		return nil, errors.New("AgentDock 节点数据库不能为空")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("初始化 AgentDock 节点加密器: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("初始化 AgentDock 节点 GCM: %w", err)
	}
	return &Store{db: db, cipher: aead, now: time.Now}, nil
}

func (s *Store) List(ctx context.Context) ([]Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.name, n.endpoint, n.enabled, n.timeout_seconds,
		n.created_at, n.updated_at, CASE WHEN secrets.node_id IS NULL THEN 0 ELSE 1 END
		FROM agentdock_nodes n
		LEFT JOIN agentdock_node_secrets secrets ON secrets.node_id = n.id
		ORDER BY n.name COLLATE NOCASE, n.id`)
	if err != nil {
		return nil, fmt.Errorf("列出 AgentDock 节点: %w", err)
	}
	defer rows.Close()

	nodes := make([]Node, 0)
	for rows.Next() {
		node, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历 AgentDock 节点: %w", err)
	}
	return nodes, nil
}

func (s *Store) Get(ctx context.Context, id string) (Node, error) {
	id, err := normalizeID(id)
	if err != nil {
		return Node{}, err
	}
	row := s.db.QueryRowContext(ctx, `SELECT n.id, n.name, n.endpoint, n.enabled, n.timeout_seconds,
		n.created_at, n.updated_at, CASE WHEN secrets.node_id IS NULL THEN 0 ELSE 1 END
		FROM agentdock_nodes n
		LEFT JOIN agentdock_node_secrets secrets ON secrets.node_id = n.id
		WHERE n.id = ?`, id)
	node, err := scanNode(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNodeNotFound
	}
	return node, err
}

func (s *Store) Credentials(ctx context.Context, id string) (Credentials, error) {
	node, err := s.Get(ctx, id)
	if err != nil {
		return Credentials{}, err
	}
	if !node.Enabled {
		return Credentials{}, ErrNodeDisabled
	}

	var sealed []byte
	if err := s.db.QueryRowContext(ctx, `SELECT token_ciphertext FROM agentdock_node_secrets WHERE node_id = ?`, node.ID).Scan(&sealed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Credentials{}, ErrNodeCredentialsAbsent
		}
		return Credentials{}, fmt.Errorf("读取 AgentDock 节点凭据: %w", err)
	}
	token, err := s.open(node.ID, sealed)
	if err != nil {
		return Credentials{}, err
	}
	return Credentials{Node: node, Token: token}, nil
}

func (s *Store) Create(ctx context.Context, input CreateInput) (Node, error) {
	id, err := normalizeID(input.ID)
	if err != nil {
		return Node{}, err
	}
	name, err := normalizeName(input.Name)
	if err != nil {
		return Node{}, err
	}
	endpoint, err := normalizeEndpoint(input.Endpoint)
	if err != nil {
		return Node{}, err
	}
	token, err := normalizeToken(input.Token)
	if err != nil {
		return Node{}, err
	}
	timeout := input.TimeoutSeconds
	if timeout == 0 {
		timeout = defaultTimeoutSeconds
	} else if timeout, err = normalizeTimeout(timeout); err != nil {
		return Node{}, err
	}
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	sealed, err := s.seal(id, token)
	if err != nil {
		return Node{}, err
	}
	now := s.now().UTC()
	timestamp := now.Format(time.RFC3339Nano)

	err = core.NewTxManager(s.db).WithinTx(ctx, nil, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO agentdock_nodes(id, name, endpoint, enabled, timeout_seconds, created_at, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?)`, id, name, endpoint, boolInt(enabled), timeout, timestamp, timestamp); err != nil {
			if isUniqueConstraint(err) {
				return ErrNodeExists
			}
			return fmt.Errorf("创建 AgentDock 节点: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO agentdock_node_secrets(node_id, token_ciphertext, updated_at)
			VALUES(?, ?, ?)`, id, sealed, timestamp); err != nil {
			return fmt.Errorf("保存 AgentDock 节点凭据: %w", err)
		}
		return nil
	})
	if err != nil {
		return Node{}, err
	}
	return Node{ID: id, Name: name, Endpoint: endpoint, Enabled: enabled, TimeoutSeconds: timeout, TokenConfigured: true, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) Update(ctx context.Context, id string, input UpdateInput) (Node, error) {
	if input.Name == nil && input.Endpoint == nil && input.Token == nil && input.Enabled == nil && input.TimeoutSeconds == nil {
		return Node{}, invalid("至少提交一个需要更新的节点字段")
	}
	id, err := normalizeID(id)
	if err != nil {
		return Node{}, err
	}
	current, err := s.Get(ctx, id)
	if err != nil {
		return Node{}, err
	}

	name := current.Name
	if input.Name != nil {
		name, err = normalizeName(*input.Name)
		if err != nil {
			return Node{}, err
		}
	}
	endpoint := current.Endpoint
	if input.Endpoint != nil {
		endpoint, err = normalizeEndpoint(*input.Endpoint)
		if err != nil {
			return Node{}, err
		}
	}
	enabled := current.Enabled
	if input.Enabled != nil {
		enabled = *input.Enabled
	}
	timeout := current.TimeoutSeconds
	if input.TimeoutSeconds != nil {
		timeout, err = normalizeTimeout(*input.TimeoutSeconds)
		if err != nil {
			return Node{}, err
		}
	}

	var sealed []byte
	if input.Token != nil {
		token, tokenErr := normalizeToken(*input.Token)
		if tokenErr != nil {
			return Node{}, tokenErr
		}
		sealed, err = s.seal(id, token)
		if err != nil {
			return Node{}, err
		}
	}
	now := s.now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	err = core.NewTxManager(s.db).WithinTx(ctx, nil, func(ctx context.Context, tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE agentdock_nodes
			SET name = ?, endpoint = ?, enabled = ?, timeout_seconds = ?, updated_at = ?
			WHERE id = ?`, name, endpoint, boolInt(enabled), timeout, timestamp, id)
		if err != nil {
			if isUniqueConstraint(err) {
				return ErrNodeExists
			}
			return fmt.Errorf("更新 AgentDock 节点: %w", err)
		}
		if affected, _ := result.RowsAffected(); affected == 0 {
			return ErrNodeNotFound
		}
		if sealed != nil {
			// 凭据表与节点元数据分离；upsert 也能修复极端情况下缺失的凭据行。
			if _, err := tx.ExecContext(ctx, `INSERT INTO agentdock_node_secrets(node_id, token_ciphertext, updated_at)
				VALUES(?, ?, ?)
				ON CONFLICT(node_id) DO UPDATE SET token_ciphertext = excluded.token_ciphertext, updated_at = excluded.updated_at`, id, sealed, timestamp); err != nil {
				return fmt.Errorf("更新 AgentDock 节点凭据: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return Node{}, err
	}
	current.Name = name
	current.Endpoint = endpoint
	current.Enabled = enabled
	current.TimeoutSeconds = timeout
	current.UpdatedAt = now
	if sealed != nil {
		current.TokenConfigured = true
	}
	return current, nil
}

func (s *Store) Delete(ctx context.Context, id string) error {
	id, err := normalizeID(id)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM agentdock_nodes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除 AgentDock 节点: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNodeNotFound
	}
	return nil
}

func (s *Store) seal(nodeID, token string) ([]byte, error) {
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("生成 AgentDock 节点加密 nonce: %w", err)
	}
	sealed := s.cipher.Seal(nil, nonce, []byte(token), []byte(nodeID))
	result := make([]byte, 1+len(nonce)+len(sealed))
	result[0] = secretVersion
	copy(result[1:], nonce)
	copy(result[1+len(nonce):], sealed)
	return result, nil
}

func (s *Store) open(nodeID string, sealed []byte) (string, error) {
	if len(sealed) <= 1+s.cipher.NonceSize() || sealed[0] != secretVersion {
		return "", errors.New("AgentDock 节点凭据格式无效")
	}
	nonceEnd := 1 + s.cipher.NonceSize()
	plain, err := s.cipher.Open(nil, sealed[1:nonceEnd], sealed[nonceEnd:], []byte(nodeID))
	if err != nil {
		return "", errors.New("AgentDock 节点凭据无法解密")
	}
	return string(plain), nil
}

func loadOrCreateKey(path string) ([]byte, error) {
	keyDir := filepath.Dir(path)
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		return nil, fmt.Errorf("创建 AgentDock 节点密钥目录: %w", err)
	}
	if err := os.Chmod(keyDir, 0o700); err != nil {
		return nil, fmt.Errorf("设置 AgentDock 节点密钥目录权限: %w", err)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) != 32 {
			return nil, errors.New("AgentDock 节点密钥长度无效")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("设置 AgentDock 节点密钥权限: %w", err)
		}
		return data, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("读取 AgentDock 节点密钥: %w", err)
	}

	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("生成 AgentDock 节点密钥: %w", err)
	}
	// 先完整同步临时文件，再通过同目录硬链接原子发布，避免崩溃留下半截密钥。
	file, err := os.CreateTemp(keyDir, ".agentdock-nodes-key-*")
	if err != nil {
		return nil, fmt.Errorf("创建 AgentDock 节点临时密钥: %w", err)
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("设置 AgentDock 节点临时密钥权限: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("写入 AgentDock 节点密钥: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("同步 AgentDock 节点密钥: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("关闭 AgentDock 节点密钥: %w", err)
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return loadOrCreateKey(path)
		}
		return nil, fmt.Errorf("发布 AgentDock 节点密钥: %w", err)
	}
	return key, nil
}

func scanNode(scanner interface{ Scan(...any) error }) (Node, error) {
	var node Node
	var enabled, tokenConfigured int
	var createdAt, updatedAt string
	if err := scanner.Scan(&node.ID, &node.Name, &node.Endpoint, &enabled, &node.TimeoutSeconds, &createdAt, &updatedAt, &tokenConfigured); err != nil {
		return Node{}, err
	}
	var err error
	if node.CreatedAt, err = parseTime(createdAt); err != nil {
		return Node{}, err
	}
	if node.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Node{}, err
	}
	node.Enabled = enabled == 1
	node.TokenConfigured = tokenConfigured == 1
	return node, nil
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析 AgentDock 节点时间: %w", err)
	}
	return parsed, nil
}

func normalizeID(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if !nodeIDPattern.MatchString(value) {
		return "", invalid("节点 ID 只能包含小写字母、数字、下划线和连字符，且长度不超过 64")
	}
	return value, nil
}

func normalizeName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 100 {
		return "", invalid("节点名称不能为空且不能超过 100 个字符")
	}
	return value, nil
}

func normalizeEndpoint(value string) (string, error) {
	value = strings.TrimSpace(value)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return "", invalid("节点地址必须是有效的 HTTP 或 HTTPS Origin")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", invalid("节点地址只支持 HTTP 或 HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", invalid("节点地址只能填写 Origin，不能包含凭据、路径、查询参数或片段")
	}
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}

func normalizeToken(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", invalid("AgentDock Token 不能为空")
	}
	if len(value) > maxTokenBytes {
		return "", invalid("AgentDock Token 过长")
	}
	return value, nil
}

func normalizeTimeout(value int) (int, error) {
	if value < 1 || value > 300 {
		return 0, invalid("请求超时必须在 1 到 300 秒之间")
	}
	return value, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func isUniqueConstraint(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}
