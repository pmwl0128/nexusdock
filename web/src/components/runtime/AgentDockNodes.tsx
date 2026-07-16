import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { Cable, CirclePlus, Pencil, RefreshCw, Server, Trash2 } from 'lucide-react';
import { api } from '../../api/client';
import Dialog from '../Dialog';

const selectedNodeStorageKey = 'nexus:runtime-node-id';

type Notice = { tone: 'success' | 'error' | 'info'; text: string };

export type AgentDockNode = {
  id: string;
  name: string;
  endpoint: string;
  enabled: boolean;
  timeout_seconds: number;
  token_configured: boolean;
  created_at: string;
  updated_at: string;
};

type NodeListResponse = { ok: boolean; nodes: AgentDockNode[]; count: number };
type NodeResponse = { ok: boolean; node: AgentDockNode };

type NodeFormState = {
  id: string;
  name: string;
  endpoint: string;
  token: string;
  timeoutSeconds: string;
  enabled: boolean;
};

const emptyForm: NodeFormState = {
  id: '',
  name: '',
  endpoint: '',
  token: '',
  timeoutSeconds: '8',
  enabled: true,
};

export function useAgentDockNodes(refreshToken: number) {
  const [nodes, setNodes] = useState<AgentDockNode[]>([]);
  const [selectedNodeID, setSelectedNodeID] = useState(() => window.localStorage.getItem(selectedNodeStorageKey) || '');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [revision, setRevision] = useState(0);

  const reload = useCallback(() => setRevision((value) => value + 1), []);
  const selectNode = useCallback((nodeID: string) => {
    setSelectedNodeID(nodeID);
    if (nodeID) window.localStorage.setItem(selectedNodeStorageKey, nodeID);
    else window.localStorage.removeItem(selectedNodeStorageKey);
  }, []);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError('');
    api<NodeListResponse>('/v1/runtime/nodes').then((result) => {
      if (cancelled) return;
      const next = result.nodes || [];
      setNodes(next);
      if (selectedNodeID && !next.some((node) => node.id === selectedNodeID && node.enabled)) selectNode('');
    }).catch((cause) => {
      if (!cancelled) setError(cause instanceof Error ? cause.message : '无法读取 AgentDock 节点');
    }).finally(() => {
      if (!cancelled) setLoading(false);
    });
    return () => { cancelled = true; };
  }, [refreshToken, revision, selectedNodeID, selectNode]);

  const selectedNode = useMemo(
    () => nodes.find((node) => node.id === selectedNodeID && node.enabled) || null,
    [nodes, selectedNodeID],
  );

  return { nodes, selectedNode, selectedNodeID, loading, error, reload, selectNode };
}

export function AgentDockNodeSelector({ nodes, selectedNodeID, onSelect }: {
  nodes: AgentDockNode[];
  selectedNodeID: string;
  onSelect: (nodeID: string) => void;
}) {
  const enabledNodes = nodes.filter((node) => node.enabled);
  return <label className="runtime-node-selector">
    <Server size={15} />
    <span>AgentDock</span>
    <select aria-label="选择 AgentDock 节点" value={selectedNodeID} onChange={(event) => onSelect(event.target.value)}>
      <option value="">请选择节点</option>
      {enabledNodes.map((node) => <option key={node.id} value={node.id}>{node.name}</option>)}
    </select>
  </label>;
}

export function AgentDockNodeRequired({ children }: { children?: ReactNode }) {
  return <section className="runtime-node-required">
    <span><Server size={25} /></span>
    <h2>请选择 AgentDock 节点</h2>
    <p>Runtime 请求必须明确指定节点。请从上方选择，或前往设置添加节点。</p>
    {children}
  </section>;
}

export function AgentDockNodesPanel({ nodes, selectedNodeID, loading, error, onReload, onSelect }: {
  nodes: AgentDockNode[];
  selectedNodeID: string;
  loading: boolean;
  error: string;
  onReload: () => void;
  onSelect: (nodeID: string) => void;
}) {
  const [editing, setEditing] = useState<AgentDockNode | 'new' | null>(null);
  const [form, setForm] = useState<NodeFormState>(emptyForm);
  const [deleting, setDeleting] = useState<AgentDockNode | null>(null);
  const [busy, setBusy] = useState('');
  const [notice, setNotice] = useState<Notice | null>(null);

  function openCreate() {
    setForm(emptyForm);
    setEditing('new');
  }

  function openEdit(node: AgentDockNode) {
    setForm({
      id: node.id,
      name: node.name,
      endpoint: node.endpoint,
      token: '',
      timeoutSeconds: String(node.timeout_seconds),
      enabled: node.enabled,
    });
    setEditing(node);
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!editing || busy) return;
    const create = editing === 'new';
    setBusy('save');
    setNotice(null);
    try {
      const payload: Record<string, unknown> = {
        name: form.name.trim(),
        endpoint: form.endpoint.trim(),
        timeout_seconds: Number(form.timeoutSeconds),
        enabled: form.enabled,
      };
      if (create) payload.id = form.id.trim();
      if (create || form.token) payload.token = form.token;
      const path = create ? '/v1/runtime/nodes' : `/v1/runtime/nodes/${encodeURIComponent(editing.id)}`;
      const result = await api<NodeResponse>(path, { method: create ? 'POST' : 'PATCH', body: JSON.stringify(payload) });
      setEditing(null);
      onReload();
      if (create) onSelect(result.node.id);
      setNotice({ tone: 'success', text: create ? `已添加 ${result.node.name}` : `已更新 ${result.node.name}` });
    } catch (cause) {
      setNotice({ tone: 'error', text: cause instanceof Error ? cause.message : '节点保存失败' });
    } finally {
      setBusy('');
    }
  }

  async function probe(node: AgentDockNode) {
    setBusy(`probe:${node.id}`);
    setNotice(null);
    try {
      await api(`/v1/runtime/nodes/${encodeURIComponent(node.id)}/probe`, { method: 'POST', timeoutMs: Math.max(15_000, node.timeout_seconds * 1000 + 2_000) });
      setNotice({ tone: 'success', text: `${node.name} 连接和 Runtime API 均正常` });
    } catch (cause) {
      setNotice({ tone: 'error', text: cause instanceof Error ? cause.message : '节点探测失败' });
    } finally {
      setBusy('');
    }
  }

  async function remove() {
    if (!deleting || busy) return;
    setBusy('delete');
    setNotice(null);
    try {
      await api(`/v1/runtime/nodes/${encodeURIComponent(deleting.id)}`, { method: 'DELETE' });
      if (selectedNodeID === deleting.id) onSelect('');
      setDeleting(null);
      onReload();
      setNotice({ tone: 'success', text: `已删除 ${deleting.name}` });
    } catch (cause) {
      setNotice({ tone: 'error', text: cause instanceof Error ? cause.message : '节点删除失败' });
    } finally {
      setBusy('');
    }
  }

  return <section className="agentdock-nodes-panel">
    <header>
      <div><span className="nexus-eyebrow">RUNTIME NODES</span><h2>AgentDock 节点</h2><p>每个节点使用独立地址和加密 Token；Runtime 操作必须明确选择节点。</p></div>
      <div className="agentdock-node-actions">
        <button type="button" className="nx-button is-secondary" onClick={onReload} disabled={loading}><RefreshCw size={15} />刷新</button>
        <button type="button" className="nx-button is-primary" onClick={openCreate}><CirclePlus size={15} />添加节点</button>
      </div>
    </header>

    {(error || notice) && <div className={`nx-alert is-${error || notice?.tone === 'error' ? 'error' : notice?.tone || 'info'}`}>{error || notice?.text}</div>}

    <div className="agentdock-node-list">
      {loading && nodes.length === 0 ? <p className="empty-mini">正在读取 AgentDock 节点…</p> : nodes.length === 0 ? <p className="empty-mini">尚未配置 AgentDock 节点。</p> : nodes.map((node) => <article key={node.id} className={selectedNodeID === node.id ? 'is-selected' : ''}>
        <span className="agentdock-node-icon"><Server size={18} /></span>
        <div className="agentdock-node-copy"><div><strong>{node.name}</strong><code>{node.id}</code>{!node.enabled && <em>已停用</em>}</div><small>{node.endpoint}</small><span>超时 {node.timeout_seconds}s · Token {node.token_configured ? '已配置' : '缺失'}</span></div>
        <div className="agentdock-node-row-actions">
          <button type="button" className="nx-button is-secondary is-small" disabled={!node.enabled || !!busy} onClick={() => void probe(node)}><Cable size={14} />{busy === `probe:${node.id}` ? '探测中' : '探测'}</button>
          <button type="button" className="nx-button is-secondary is-small" disabled={!!busy} onClick={() => openEdit(node)}><Pencil size={14} />编辑</button>
          <button type="button" className="nx-button is-danger is-small" disabled={!!busy} onClick={() => setDeleting(node)}><Trash2 size={14} />删除</button>
        </div>
      </article>)}
    </div>

    {editing && <Dialog title={editing === 'new' ? '添加 AgentDock 节点' : `编辑 ${editing.name}`} description="地址只填写 Origin，不包含 /mcp 或其他路径。Token 保存后不会再次显示。" onClose={() => setEditing(null)} wide>
      <form className="agentdock-node-form" onSubmit={submit}>
        <label><span>节点 ID</span><input required pattern="[a-z0-9][a-z0-9_-]{0,63}" value={form.id} disabled={editing !== 'new'} onChange={(event) => setForm({ ...form, id: event.target.value.toLowerCase() })} placeholder="dockmini" /></label>
        <label><span>显示名称</span><input required maxLength={100} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="DockMini" /></label>
        <label className="is-wide"><span>AgentDock 地址</span><input required type="url" value={form.endpoint} onChange={(event) => setForm({ ...form, endpoint: event.target.value })} placeholder="https://dockmini.example.com" /></label>
        <label className="is-wide"><span>{editing === 'new' ? 'Bearer Token' : '替换 Token（留空则保持不变）'}</span><input required={editing === 'new'} type="password" autoComplete="new-password" value={form.token} onChange={(event) => setForm({ ...form, token: event.target.value })} /></label>
        <label><span>请求超时（秒）</span><input required type="number" min={1} max={300} value={form.timeoutSeconds} onChange={(event) => setForm({ ...form, timeoutSeconds: event.target.value })} /></label>
        <label className="agentdock-node-check"><input type="checkbox" checked={form.enabled} onChange={(event) => setForm({ ...form, enabled: event.target.checked })} /><span>启用节点</span></label>
        <footer><button type="button" className="nx-button is-secondary" onClick={() => setEditing(null)}>取消</button><button type="submit" className="nx-button is-primary" disabled={busy === 'save'}>{busy === 'save' ? '保存中…' : '保存节点'}</button></footer>
      </form>
    </Dialog>}

    {deleting && <Dialog title="删除 AgentDock 节点" description="节点配置和加密凭据将被永久删除。AgentDock 服务本身不会被卸载。" onClose={() => setDeleting(null)}>
      <div className="agentdock-node-delete"><p>确定删除「{deleting.name}」？</p><code>{deleting.endpoint}</code><footer><button type="button" className="nx-button is-secondary" onClick={() => setDeleting(null)}>取消</button><button type="button" className="nx-button is-danger" disabled={busy === 'delete'} onClick={() => void remove()}>{busy === 'delete' ? '删除中…' : '确认删除'}</button></footer></div>
    </Dialog>}
  </section>;
}
