import { FileText, Plus, Search } from 'lucide-react';
import type { RecallWorkspaceViewModel } from './types';
import { formatBytes, nameOf } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'fileEntries' | 'actions'>;

export default function RecallFileBrowser({ state, fileEntries, actions }: Props) {
  return <aside className="mem-lite-browser">
    <div className="mem-lite-panel-head">
      <div><h2>文件</h2><p>{state.query ? '搜索结果' : '当前召回仓库'}</p></div>
      <button type="button" className="icon" onClick={actions.startNew} title="新建召回条目"><Plus size={17} /></button>
    </div>
    <form className="mem-lite-search" onSubmit={actions.searchMemories}>
      <Search size={15} />
      <input aria-label="搜索召回内容" value={state.query} onChange={(event) => actions.setQuery(event.target.value)} placeholder="搜索召回内容" />
      <button type="submit">搜索</button>
    </form>
    <div className="mem-lite-files">
      {state.loading ? <p className="mem-lite-empty">正在读取召回内容…</p> : fileEntries.length === 0 ? <p className="mem-lite-empty">没有匹配的召回文件。</p> : fileEntries.map((entry) => (
        <button type="button" key={entry.path} className={state.current?.path === entry.path ? 'active' : ''} onClick={() => actions.openRecall(entry.path)}>
          <FileText size={16} />
          <span><strong>{nameOf(entry.path)}</strong><small>{entry.path}</small></span>
          <em>{formatBytes(entry.size_bytes)}</em>
        </button>
      ))}
    </div>
  </aside>;
}
