import { FileText, Sparkles } from 'lucide-react';
import type { RecallWorkspaceViewModel } from './types';
import { nameOf } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'actions'> & {
  onOpenCard: (path: string) => void;
};

export default function RecallExperienceCardsPage({ state, actions, onOpenCard }: Props) {
  const { card, busy } = state;
  return <section className="recall-tool-page">
    <article className="recall-tool-panel">
      <div className="recall-panel-head">
        <div><h2>经验卡片</h2><p>生成候选、检查相似经验，再确认写入受管卡片目录。</p></div>
        <span className={`recall-health ${card.capture?.warnings?.length ? 'warn' : 'ok'}`}>{card.capture ? '候选已生成' : '待捕获'}</span>
      </div>
      <form className="recall-card-form" onSubmit={actions.captureCard}>
        <label><span>标题</span><input value={card.title} onChange={(event) => actions.setCardField('title', event.target.value)} placeholder="例如：Recall BGE-M3 标准入口" /></label>
        <label><span>项目</span><input value={card.project} onChange={(event) => actions.setCardField('project', event.target.value)} /></label>
        <label><span>类型</span><select value={card.type} onChange={(event) => actions.setCardField('type', event.target.value)}><option value="runbook">runbook</option><option value="bug_pattern">bug_pattern</option><option value="deploy_note">deploy_note</option><option value="project_trap">project_trap</option><option value="architecture">architecture</option><option value="decision">decision</option><option value="anti_pattern">anti_pattern</option><option value="preference">preference</option></select></label>
        <label><span>来源</span><input value={card.source} onChange={(event) => actions.setCardField('source', event.target.value)} /></label>
        <label className="wide"><span>内容</span><textarea value={card.content} onChange={(event) => actions.setCardField('content', event.target.value)} placeholder="写可复用结论，不写临时日志。" /></label>
        <label><span>证据</span><input value={card.evidence} onChange={(event) => actions.setCardField('evidence', event.target.value)} placeholder="命令、端点、commit 或验证结果" /></label>
        <label><span>标签</span><input value={card.tags} onChange={(event) => actions.setCardField('tags', event.target.value)} placeholder="逗号分隔" /></label>
        <label className="wide"><span>目标路径</span><input value={card.path} onChange={(event) => actions.setCardField('path', event.target.value)} placeholder="留空时自动生成 cards/<project>/inbox/<type>/..." /></label>
        <div className="recall-card-actions">
          <button type="submit" disabled={busy}><Sparkles size={15} />生成候选</button>
          <button type="button" className="primary" onClick={actions.writeCard} disabled={busy || !card.capture}>确认写入</button>
          {card.capture?.warnings?.length ? <label className="recall-card-check"><input type="checkbox" checked={card.allowWarnings} onChange={(event) => actions.setAllowCardWarnings(event.target.checked)} />已人工确认警告</label> : null}
        </div>
      </form>
      {card.capture && <div className="recall-card-review"><strong>{card.capture.card.path}</strong>{card.capture.warnings?.map((warning) => <p key={warning} className="warn">{warning}</p>)}{(card.capture.similar_results || []).map((item) => <button key={item.path} type="button" onClick={() => onOpenCard(item.path)}><FileText size={14} /><span>{item.title || nameOf(item.path)}</span><em>{item.path}</em></button>)}</div>}
    </article>
  </section>;
}
