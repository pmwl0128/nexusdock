import { useEffect, useState, type FormEvent } from 'react';
import { Activity, BrainCircuit, DatabaseZap, RefreshCw, Save, SearchCheck } from 'lucide-react';
import { ApiError, api } from '../../api/client';

type SecretForm = { value: string; clear: boolean };
type EmbeddingSettings = {
  enabled: boolean;
  endpoint: string;
  model: string;
  timeout_seconds: number;
  api_key_configured: boolean;
};
type Stage3Settings = {
  enabled: boolean;
  endpoint: string;
  model: string;
  timeout_seconds: number;
  interval_minutes: number;
  api_key_configured: boolean;
  configured: boolean;
};
type RuntimeAISettings = {
  embedding: EmbeddingSettings;
  stage3: Stage3Settings;
  persisted: boolean;
  updated_at?: string;
};
type SettingsResponse = { ok: boolean; settings: RuntimeAISettings };
type ConnectionTestResult = {
  ok: boolean;
  target: 'stage3' | 'embedding';
  model?: string;
  message: string;
  latency_ms: number;
};
type EmbeddingStatus = {
  ok: boolean;
  enabled: boolean;
  configured: boolean;
  reachable?: boolean;
  model?: string;
  error?: string;
  reason?: string;
  index?: { count?: number; dimension?: number; updated_at?: string };
};

type FormState = {
  embedding: EmbeddingSettings;
  stage3: Stage3Settings;
};

const emptySettings: RuntimeAISettings = {
  embedding: { enabled: false, endpoint: '', model: 'BAAI/bge-m3', timeout_seconds: 30, api_key_configured: false },
  stage3: { enabled: false, endpoint: '', model: '', timeout_seconds: 60, interval_minutes: 360, api_key_configured: false, configured: false },
  persisted: false,
};

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  return error instanceof Error ? error.message : '请求失败';
}

function secretAction(secret: SecretForm) {
  if (secret.clear) return { action: 'clear' };
  if (secret.value.trim()) return { action: 'replace', value: secret.value.trim() };
  return { action: 'keep' };
}

export default function AISettingsPanel({ refreshToken }: { refreshToken: number }) {
  const [form, setForm] = useState<FormState>({ embedding: emptySettings.embedding, stage3: emptySettings.stage3 });
  const [embeddingSecret, setEmbeddingSecret] = useState<SecretForm>({ value: '', clear: false });
  const [stage3Secret, setStage3Secret] = useState<SecretForm>({ value: '', clear: false });
  const [embeddingStatus, setEmbeddingStatus] = useState<EmbeddingStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [reindexing, setReindexing] = useState(false);
  const [testingTarget, setTestingTarget] = useState<'stage3' | 'embedding' | null>(null);
  const [stage3Test, setStage3Test] = useState<ConnectionTestResult | null>(null);
  const [embeddingTest, setEmbeddingTest] = useState<ConnectionTestResult | null>(null);
  const [notice, setNotice] = useState<{ tone: 'success' | 'error' | 'info'; text: string } | null>(null);

  async function refreshEmbeddingStatus() {
    try {
      setEmbeddingStatus(await api<EmbeddingStatus>('/v1/embeddings/status', { timeoutMs: 35_000 }));
    } catch (error) {
      setEmbeddingStatus({ ok: false, enabled: form.embedding.enabled, configured: false, reachable: false, error: errorMessage(error) });
    }
  }

  async function load() {
    setLoading(true);
    try {
      const settingsResult = await api<SettingsResponse>('/v1/settings/ai');
      setForm({ embedding: settingsResult.settings.embedding, stage3: settingsResult.settings.stage3 });
      setEmbeddingSecret({ value: '', clear: false });
      setStage3Secret({ value: '', clear: false });
      setStage3Test(null);
      setEmbeddingTest(null);
      setNotice(null);
      void refreshEmbeddingStatus();
    } catch (error) {
      setNotice({ tone: 'error', text: errorMessage(error) });
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, [refreshToken]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setNotice(null);
    try {
      const result = await api<SettingsResponse>('/v1/settings/ai', {
        method: 'PUT',
        body: JSON.stringify({
          embedding: {
            enabled: form.embedding.enabled,
            endpoint: form.embedding.endpoint.trim(),
            model: form.embedding.model.trim(),
            timeout_seconds: form.embedding.timeout_seconds,
            api_key: secretAction(embeddingSecret),
          },
          stage3: {
            enabled: form.stage3.enabled,
            endpoint: form.stage3.endpoint.trim(),
            model: form.stage3.model.trim(),
            timeout_seconds: form.stage3.timeout_seconds,
            interval_minutes: form.stage3.interval_minutes,
            api_key: secretAction(stage3Secret),
          },
        }),
      });
      setForm({ embedding: result.settings.embedding, stage3: result.settings.stage3 });
      setEmbeddingSecret({ value: '', clear: false });
      setStage3Secret({ value: '', clear: false });
      setStage3Test(null);
      setEmbeddingTest(null);
      setNotice({ tone: 'success', text: '配置已保存并应用，无需重启 Nexus。' });
      void refreshEmbeddingStatus();
    } catch (error) {
      setNotice({ tone: 'error', text: errorMessage(error) });
    } finally {
      setSaving(false);
    }
  }

  async function reindex() {
    setReindexing(true);
    setNotice(null);
    try {
      await api('/v1/embeddings/reindex', { method: 'POST', body: '{}' , timeoutMs: 120_000 });
      await api('/v1/workflow-templates/reindex', { method: 'POST', timeoutMs: 120_000 });
      setNotice({ tone: 'success', text: 'Recall 与 Workflow 向量索引已重建。' });
      void refreshEmbeddingStatus();
    } catch (error) {
      setNotice({ tone: 'error', text: errorMessage(error) });
    } finally {
      setReindexing(false);
    }
  }

  async function testConnection(target: 'stage3' | 'embedding') {
    setTestingTarget(target);
    const setResult = target === 'stage3' ? setStage3Test : setEmbeddingTest;
    setResult(null);
    try {
      const result = await api<ConnectionTestResult>(`/v1/settings/ai/test/${target}`, {
        method: 'POST',
        timeoutMs: 310_000,
      });
      setResult(result);
    } catch (error) {
      setResult({ ok: false, target, message: errorMessage(error), latency_ms: 0 });
    } finally {
      setTestingTarget(null);
    }
  }

  const reachableTone = embeddingStatus?.reachable === true ? 'is-ok' : embeddingStatus?.enabled ? 'is-warn' : 'is-muted';
  return <section className="ai-settings-panel">
    <header className="ai-settings-heading">
      <div><span className="nexus-eyebrow">AI & VECTOR</span><h2>模型与向量检索</h2><p>Stage 3 使用外部大模型做低频补漏；Recall 与 Workflow 共用一套 Embedding 配置。</p></div>
      <button type="button" className="nx-button is-secondary" onClick={() => void load()} disabled={loading || saving}><RefreshCw size={15} />刷新</button>
    </header>
    {notice && <div className={`nx-alert is-${notice.tone}`}>{notice.text}</div>}

    <form className="ai-settings-grid" onSubmit={submit}>
      <article className="nexus-panel ai-settings-card">
        <header><span className="nexus-panel-icon"><BrainCircuit size={17} /></span><div><h3>Stage 3 大模型</h3><p>跨 Task / Evolution 的低频语义补漏，只提交候选给 AgentDock 裁决。</p></div></header>
        <div className="panel-body ai-settings-form">
          <label className="ai-toggle"><input type="checkbox" checked={form.stage3.enabled} onChange={(event) => setForm({ ...form, stage3: { ...form.stage3, enabled: event.target.checked } })} /><span><strong>启用 Stage 3</strong><small>{form.stage3.configured ? '当前已具备运行配置' : '关闭时不会调用外部模型'}</small></span></label>
          <label className="is-wide"><span>Chat Completions 地址</span><input type="url" required={form.stage3.enabled} value={form.stage3.endpoint} onChange={(event) => setForm({ ...form, stage3: { ...form.stage3, endpoint: event.target.value } })} placeholder="https://api.example.com/v1/chat/completions" /></label>
          <label><span>模型</span><input required={form.stage3.enabled} value={form.stage3.model} onChange={(event) => setForm({ ...form, stage3: { ...form.stage3, model: event.target.value } })} placeholder="gpt-5-mini" /></label>
          <label><span>请求超时（秒）</span><input type="number" min={1} max={300} value={form.stage3.timeout_seconds} onChange={(event) => setForm({ ...form, stage3: { ...form.stage3, timeout_seconds: Number(event.target.value) } })} /></label>
          <label><span>执行间隔（分钟）</span><input type="number" min={60} max={10080} value={form.stage3.interval_minutes} onChange={(event) => setForm({ ...form, stage3: { ...form.stage3, interval_minutes: Number(event.target.value) } })} /></label>
          <label className="is-wide"><span>API Key {form.stage3.api_key_configured ? '· 已配置（留空保持）' : '· 未配置'}</span><input type="password" autoComplete="new-password" disabled={stage3Secret.clear} value={stage3Secret.value} onChange={(event) => setStage3Secret({ value: event.target.value, clear: false })} placeholder={form.stage3.api_key_configured ? '••••••••' : '可选'} /></label>
          {form.stage3.api_key_configured && <label className="ai-clear-secret"><input type="checkbox" checked={stage3Secret.clear} onChange={(event) => setStage3Secret({ value: '', clear: event.target.checked })} /><span>清除已保存的 API Key</span></label>}
          <div className="ai-card-actions"><button type="button" className="nx-button is-secondary" disabled={loading || saving || testingTarget !== null} onClick={() => void testConnection('stage3')}><Activity size={15} />{testingTarget === 'stage3' ? '测试中…' : '测试已保存配置'}</button><small>测试不会保存当前表单；修改后请先“保存并应用”。</small></div>
          {stage3Test && <div className={`nx-alert is-${stage3Test.ok ? 'success' : 'error'}`}>{stage3Test.message}{stage3Test.latency_ms > 0 ? ` · ${stage3Test.latency_ms} ms` : ''}</div>}
        </div>
      </article>

      <article className="nexus-panel ai-settings-card">
        <header><span className="nexus-panel-icon"><DatabaseZap size={17} /></span><div><h3>向量检索</h3><p>用于 Recall 语义搜索和 Workflow 模板匹配，兼容 OpenAI Embeddings API。</p></div></header>
        <div className="panel-body ai-settings-form">
          <div className="ai-vector-status">
            <span className={`ai-status-dot ${reachableTone}`} />
            <span><strong>{embeddingStatus?.enabled ? (embeddingStatus.reachable ? '向量服务可用' : '向量服务不可达') : '向量检索未启用'}</strong><small>{embeddingStatus?.enabled ? (embeddingStatus.index ? `索引 ${embeddingStatus.index.count ?? 0} 项 · ${embeddingStatus.index.dimension ?? 0} 维` : '已启用，等待索引状态') : '当前已关闭'}</small></span>
          </div>
          <label className="ai-toggle"><input type="checkbox" checked={form.embedding.enabled} onChange={(event) => setForm({ ...form, embedding: { ...form.embedding, enabled: event.target.checked } })} /><span><strong>启用向量检索</strong><small>Recall 与 Workflow 共用此配置</small></span></label>
          <label className="is-wide"><span>Embeddings 地址</span><input type="url" required={form.embedding.enabled} value={form.embedding.endpoint} onChange={(event) => setForm({ ...form, embedding: { ...form.embedding, endpoint: event.target.value } })} placeholder="http://embedding-service:8000/v1/embeddings" /></label>
          <label><span>Embedding 模型</span><input required={form.embedding.enabled} value={form.embedding.model} onChange={(event) => setForm({ ...form, embedding: { ...form.embedding, model: event.target.value } })} placeholder="BAAI/bge-m3" /></label>
          <label><span>请求超时（秒）</span><input type="number" min={1} max={300} value={form.embedding.timeout_seconds} onChange={(event) => setForm({ ...form, embedding: { ...form.embedding, timeout_seconds: Number(event.target.value) } })} /></label>
          <label className="is-wide"><span>API Key {form.embedding.api_key_configured ? '· 已配置（留空保持）' : '· 未配置'}</span><input type="password" autoComplete="new-password" disabled={embeddingSecret.clear} value={embeddingSecret.value} onChange={(event) => setEmbeddingSecret({ value: event.target.value, clear: false })} placeholder={form.embedding.api_key_configured ? '••••••••' : '本地 Embedding 可留空'} /></label>
          {form.embedding.api_key_configured && <label className="ai-clear-secret"><input type="checkbox" checked={embeddingSecret.clear} onChange={(event) => setEmbeddingSecret({ value: '', clear: event.target.checked })} /><span>清除已保存的 API Key</span></label>}
          <div className="ai-card-actions"><button type="button" className="nx-button is-secondary" disabled={loading || saving || testingTarget !== null} onClick={() => void testConnection('embedding')}><Activity size={15} />{testingTarget === 'embedding' ? '测试中…' : '测试已保存配置'}</button><button type="button" className="nx-button is-secondary" disabled={!form.embedding.enabled || reindexing || saving || testingTarget !== null} onClick={() => void reindex()}><SearchCheck size={15} />{reindexing ? '重建中…' : '重建 Recall + Workflow 索引'}</button></div>
          {embeddingTest && <div className={`nx-alert is-${embeddingTest.ok ? 'success' : 'error'}`}>{embeddingTest.message}{embeddingTest.latency_ms > 0 ? ` · ${embeddingTest.latency_ms} ms` : ''}</div>}
          {embeddingStatus?.error && <div className="nx-alert is-error">{embeddingStatus.error}</div>}
        </div>
      </article>

      <footer className="ai-settings-actions"><span>API Key 只返回“已配置”状态，明文不会从服务端读取。</span><button type="submit" className="nx-button" disabled={loading || saving}><Save size={15} />{saving ? '保存中…' : '保存并应用'}</button></footer>
    </form>
  </section>;
}
