import { useEffect, useMemo, useState, type FormEvent } from 'react';
import {
  ArrowRight,
  CheckCircle2,
  Clock3,
  KeyRound,
  Laptop,
  LockKeyhole,
  LogOut,
  RefreshCw,
  ShieldCheck,
  Smartphone,
  Sparkles,
  Trash2,
} from 'lucide-react';
import { ApiError, api, clearCSRFToken, setCSRFToken } from './api/client';
import './auth.css';

export type WebSession = {
  id: string;
  user_id: string;
  username: string;
  display_name: string;
  remember_me: boolean;
  ip_prefix: string;
  user_agent_summary: string;
  created_at: string;
  last_seen_at: string;
  idle_expires_at: string;
  absolute_expires_at: string;
  must_change_password: boolean;
  csrf_token?: string;
  current?: boolean;
};

type SessionResponse = { ok: boolean; session: WebSession };
type SessionsResponse = { ok: boolean; items: WebSession[] };

function safeReturnTo(raw: string | null): string {
  if (!raw || !raw.startsWith('/') || raw.startsWith('//') || /[\r\n]/.test(raw)) return '/ui/';
  try {
    const value = new URL(raw, window.location.origin);
    return value.origin === window.location.origin ? `${value.pathname}${value.search}${value.hash}` : '/ui/';
  } catch {
    return '/ui/';
  }
}

function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.code === 'INVALID_CREDENTIALS') return '用户名或密码不正确。';
    if (error.code === 'LOGIN_RATE_LIMITED') return '尝试次数过多，请稍后再试。';
    if (error.code === 'ADMIN_NOT_INITIALIZED') return '管理员尚未初始化，请在 DockMini 本机执行初始化命令。';
    if (error.code === 'HTTPS_REQUIRED') return '登录仅允许通过 HTTPS 安全连接。';
    if (error.code === 'CURRENT_CREDENTIAL_INVALID') return '当前密码不正确。';
    if (error.code === 'CREDENTIAL_POLICY_FAILED') return error.message;
    return error.message;
  }
  return error instanceof Error ? error.message : '请求失败，请稍后重试。';
}

export function LoginPage() {
  const params = useMemo(() => new URLSearchParams(window.location.search), []);
  const returnTo = safeReturnTo(params.get('return_to'));
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [rememberMe, setRememberMe] = useState(false);
  const [initialized, setInitialized] = useState<boolean | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const status = await api<{ ok: boolean; initialized: boolean }>('/v1/auth/status');
        if (!cancelled) setInitialized(status.initialized);
        if (!status.initialized) return;
        try {
          const current = await api<SessionResponse>('/v1/auth/session');
          if (current.session.csrf_token) setCSRFToken(current.session.csrf_token);
          window.location.replace(current.session.must_change_password
            ? `/change-password?return_to=${encodeURIComponent(returnTo)}`
            : returnTo);
        } catch (sessionError) {
          if (!(sessionError instanceof ApiError) || sessionError.status !== 401) throw sessionError;
        }
      } catch (loadError) {
        if (!cancelled) setError(errorMessage(loadError));
      }
    })();
    return () => { cancelled = true; };
  }, [returnTo]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!initialized || submitting) return;
    setSubmitting(true);
    setError('');
    try {
      const result = await api<SessionResponse>('/v1/auth/login', {
        method: 'POST',
        body: JSON.stringify({ username, password, remember_me: rememberMe }),
      });
      if (result.session.csrf_token) setCSRFToken(result.session.csrf_token);
      setPassword('');
      window.location.assign(result.session.must_change_password
        ? `/change-password?return_to=${encodeURIComponent(returnTo)}`
        : returnTo);
    } catch (submitError) {
      setError(errorMessage(submitError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-shell">
      <section className="auth-story" aria-label="AgentDock Nexus">
        <div className="auth-brand"><span><Sparkles size={20} /></span><strong>AgentDock Nexus</strong></div>
        <div className="auth-story-copy">
          <p className="auth-kicker">PRIVATE CONTROL PLANE</p>
          <h1>一处连接设备、记忆与自动化。</h1>
          <p>使用管理员会话进入 Nexus。Agent、设备和脚本继续通过各自的受限 Token 工作。</p>
        </div>
        <div className="auth-security-note"><ShieldCheck size={18} /><span>HttpOnly Session · SameSite Strict · CSRF 防护</span></div>
      </section>
      <section className="auth-panel">
        <form className="auth-card" onSubmit={submit}>
          <header><span className="auth-card-icon"><LockKeyhole size={21} /></span><div><h2>登录控制台</h2><p>使用 Nexus 管理员账号继续</p></div></header>
          {params.get('changed') === '1' && <div className="auth-success"><CheckCircle2 size={17} />密码已更新，请重新登录。</div>}
          {initialized === false && <div className="auth-error">管理员尚未初始化。请在 DockMini 本机运行管理命令后刷新。</div>}
          {error && <div className="auth-error" role="alert">{error}</div>}
          <label><span>用户名</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} disabled={submitting || initialized === false} required autoFocus /></label>
          <label><span>密码</span><input type="password" autoComplete="current-password" value={password} onChange={(event) => setPassword(event.target.value)} disabled={submitting || initialized === false} required /></label>
          <label className="auth-check"><input type="checkbox" checked={rememberMe} onChange={(event) => setRememberMe(event.target.checked)} /><span>记住我 30 天</span></label>
          <button className="auth-primary" type="submit" disabled={submitting || initialized !== true || !username || !password}>
            {submitting ? <><RefreshCw className="spin" size={17} />正在验证</> : <>进入 Nexus<ArrowRight size={17} /></>}
          </button>
          <p className="auth-help">忘记密码时，请在 DockMini 本机使用管理员恢复命令。Nexus 不提供公网找回入口。</p>
        </form>
      </section>
    </main>
  );
}

export function CredentialUpdatePage() {
  const params = useMemo(() => new URLSearchParams(window.location.search), []);
  const returnTo = safeReturnTo(params.get('return_to'));
  const [current, setCurrent] = useState('');
  const [next, setNext] = useState('');
  const [confirm, setConfirm] = useState('');
  const [session, setSession] = useState<WebSession | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    api<SessionResponse>('/v1/auth/session').then((result) => {
      setSession(result.session);
      if (result.session.csrf_token) setCSRFToken(result.session.csrf_token);
    }).catch(() => window.location.replace(`/login?return_to=${encodeURIComponent(returnTo)}`));
  }, [returnTo]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (next !== confirm) {
      setError('两次输入的新密码不一致。');
      return;
    }
    if (next.length < 12) {
      setError('新密码至少需要 12 个字符。');
      return;
    }
    setSubmitting(true);
    setError('');
    try {
      await api('/v1/auth/credential', { method: 'POST', body: JSON.stringify({ current, next }) });
      clearCSRFToken();
      window.location.replace(`/login?changed=1&return_to=${encodeURIComponent(returnTo)}`);
    } catch (submitError) {
      setError(errorMessage(submitError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="auth-shell auth-shell-single">
      <section className="auth-panel">
        <form className="auth-card" onSubmit={submit}>
          <header><span className="auth-card-icon"><KeyRound size={21} /></span><div><h2>{session?.must_change_password ? '首次登录安全更新' : '修改管理员密码'}</h2><p>更新后所有浏览器会话都会立即退出</p></div></header>
          {error && <div className="auth-error" role="alert">{error}</div>}
          <label><span>当前密码</span><input type="password" autoComplete="current-password" value={current} onChange={(event) => setCurrent(event.target.value)} required /></label>
          <label><span>新密码</span><input type="password" autoComplete="new-password" value={next} onChange={(event) => setNext(event.target.value)} minLength={12} required /></label>
          <label><span>确认新密码</span><input type="password" autoComplete="new-password" value={confirm} onChange={(event) => setConfirm(event.target.value)} minLength={12} required /></label>
          <p className="auth-policy">至少 12 个字符；支持密码管理器生成的长密码或密码短语。</p>
          <button className="auth-primary" type="submit" disabled={submitting || !session || !current || !next || !confirm}>{submitting ? '正在更新…' : '更新并重新登录'}</button>
        </form>
      </section>
    </main>
  );
}

export function AccountSecurity() {
  const [session, setSession] = useState<WebSession | null>(null);
  const [sessions, setSessions] = useState<WebSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  async function load() {
    setLoading(true);
    setError('');
    try {
      const [current, list] = await Promise.all([
        api<SessionResponse>('/v1/auth/session'),
        api<SessionsResponse>('/v1/auth/sessions'),
      ]);
      setSession(current.session);
      setSessions(list.items || []);
      if (current.session.csrf_token) setCSRFToken(current.session.csrf_token);
    } catch (loadError) {
      setError(errorMessage(loadError));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { void load(); }, []);

  async function revoke(id: string) {
    await api(`/v1/auth/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' });
    await load();
  }

  async function logoutOthers() {
    await api('/v1/auth/sessions/logout-others', { method: 'POST', body: '{}' });
    await load();
  }

  async function logout() {
    await api('/v1/auth/logout', { method: 'POST', body: '{}' });
    clearCSRFToken();
    window.location.assign('/login');
  }

  return (
    <section className="security-panel">
      <header className="security-head"><div><span className="security-icon"><ShieldCheck size={19} /></span><div><h2>账号与会话</h2><p>管理当前管理员会话与登录设备</p></div></div><button className="security-secondary" onClick={() => void load()} disabled={loading}><RefreshCw className={loading ? 'spin' : ''} size={15} />刷新</button></header>
      {error && <div className="auth-error">{error}</div>}
      <div className="security-profile"><div><strong>{session?.display_name || session?.username || 'Administrator'}</strong><span>{session?.username || '—'}</span></div><div className="security-actions"><button onClick={() => window.location.assign('/change-password?return_to=%2Fui%2F%23settings')}><KeyRound size={15} />修改密码</button><button onClick={() => void logout()}><LogOut size={15} />退出登录</button></div></div>
      <div className="session-toolbar"><div><strong>活动会话</strong><span>{sessions.length} 个</span></div><button onClick={() => void logoutOthers()} disabled={sessions.filter((item) => !item.current).length === 0}>退出其他设备</button></div>
      <div className="session-list">
        {sessions.map((item) => (
          <article className={`session-row ${item.current ? 'is-current' : ''}`} key={item.id}>
            <span className="session-device">{/iOS|Android/.test(item.user_agent_summary) ? <Smartphone size={18} /> : <Laptop size={18} />}</span>
            <div className="session-copy"><div><strong>{item.user_agent_summary || '未知设备'}</strong>{item.current && <em>当前</em>}</div><span>{item.ip_prefix || '未知网络'} · {item.remember_me ? '记住我' : '浏览器会话'}</span><small><Clock3 size={12} />最近活动 {formatSessionTime(item.last_seen_at)}</small></div>
            {!item.current && <button className="session-revoke" title="撤销会话" onClick={() => void revoke(item.id)}><Trash2 size={16} /></button>}
          </article>
        ))}
        {!loading && sessions.length === 0 && <p className="session-empty">没有可显示的活动会话。</p>}
      </div>
    </section>
  );
}

function formatSessionTime(value: string): string {
  if (!value) return '未知';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false });
}
