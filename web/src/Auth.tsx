import { formatTime } from './lib/time';
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
import { ApiError, api, setCSRFToken } from './api/client';
import { errorMessage, safeReturnTo } from './authShared';
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
          <label><span>用户名</span><input autoComplete="username" value={username} onChange={(event) => setUsername(event.target.value)} disabled={submitting || initialized === false} required /></label>
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
