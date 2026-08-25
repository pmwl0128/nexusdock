import { Cpu, FileText, Search, Sparkles } from 'lucide-react';
import type { RecallWorkspaceViewModel } from './types';
import { nameOf } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'actions'>;

export default function RecallCardsConsole({ state, actions }: Props) {
  const { card, embedding, busy } = state;
  return <section className="recall-card-console">
    <article className="recall-card-panel">
      <div className="recall-panel-head">
        <div><h2>经验卡片</h2><p>先生成候选和相似检查，再确认写入 cards/。</p></div>
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
      {card.capture && <div className="recall-card-review"><strong>{card.capture.card.path}</strong>{card.capture.warnings?.map((warning) => <p key={warning} className="warn">{warning}</p>)}{(card.capture.similar_results || []).map((item) => <button key={item.path} type="button" onClick={() => actions.openSimilarCard(item.path)}><FileText size={14} /><span>{item.title || nameOf(item.path)}</span><em>{item.path}</em></button>)}</div>}
    </article>

    <article className="recall-card-panel">
      <div className="recall-panel-head">
        <div><h2>向量召回</h2><p>只搜索 cards/，用于高信噪比经验召回。</p></div>
        <span className={`recall-health ${embedding.status?.reachable ? 'ok' : 'warn'}`}>{embedding.status?.reachable ? 'BGE-M3 可达' : '未就绪'}</span>
      </div>
      <div className="recall-card-embed-status">
        <div><span>模型</span><strong>{embedding.status?.model || '未知'}</strong></div>
        <div><span>维度</span><strong>{String(embedding.status?.dimension || '—')}</strong></div>
        <div><span>索引</span><strong>{String(embedding.status?.count || '—')}</strong></div>
      </div>
      <form className="recall-card-search" onSubmit={actions.searchCardEmbeddings}>
        <Search size={15} />
        <input aria-label="搜索经验卡片" value={embedding.query} onChange={(event) => actions.setEmbeddingQuery(event.target.value)} placeholder="搜索经验卡片" />
        <button type="submit" disabled={busy}>向量搜索</button>
        <button type="button" onClick={actions.reindexCards} disabled={busy}><Cpu size={15} />重建索引</button>
      </form>
      <div className="recall-card-results">
        {embedding.results.length === 0 ? <p className="recall-empty">暂无向量搜索结果。</p> : embedding.results.map((item) => <button key={item.path} type="button" onClick={() => actions.openSimilarCard(item.path)}><strong>{item.title || nameOf(item.path)}</strong><small>{item.path}</small><em>{item.score.toFixed(4)}</em></button>)}
      </div>
    </article>
  </section>;
}
