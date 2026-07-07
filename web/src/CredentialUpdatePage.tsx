import { useEffect, useMemo, useState, type FormEvent } from 'react';
import { KeyRound } from 'lucide-react';
import { api, clearCSRFToken, setCSRFToken } from './api/client';
import { type WebSession } from './Auth';
import { errorMessage, safeReturnTo } from './authShared';

type SessionResponse = { ok: boolean; session: WebSession };

export default function CredentialUpdatePage() {
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
