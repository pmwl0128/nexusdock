import { useMemo } from 'react';
import { FileText, Folder, Plus, Search } from 'lucide-react';
import type { RecallEntry, RecallWorkspaceViewModel } from './types';
import { formatBytes, nameOf, normalizePath } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'fileEntries' | 'actions'>;

type RecallFileGroup = {
  folderPath: string;
  folderName: string;
  files: RecallEntry[];
  totalSizeBytes: number;
};

function folderOf(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

function groupFileEntries(entries: RecallEntry[]): RecallFileGroup[] {
  const groups = new Map<string, RecallFileGroup>();

  for (const entry of entries) {
    const folderPath = folderOf(entry.path);
    const group = groups.get(folderPath) ?? {
      folderPath,
      folderName: folderPath ? nameOf(folderPath) : '根目录',
      files: [],
      totalSizeBytes: 0,
    };
    group.files.push(entry);
    group.totalSizeBytes += entry.size_bytes ?? 0;
    groups.set(folderPath, group);
  }

  return [...groups.values()].sort((a, b) => {
    if (!a.folderPath) return -1;
    if (!b.folderPath) return 1;
    return a.folderPath.localeCompare(b.folderPath, 'zh-CN');
  });
}

export default function RecallFileBrowser({ state, fileEntries, actions }: Props) {
  const fileGroups = useMemo(() => groupFileEntries(fileEntries), [fileEntries]);
  const resultSummary = state.query
    ? `${fileEntries.length} 个结果 / ${fileGroups.length} 个文件夹`
    : `${fileGroups.length} 个文件夹 / ${fileEntries.length} 个文件`;

  return <aside className="mem-lite-browser">
    <div className="mem-lite-panel-head">
      <div><h2>文件</h2><p>{state.query ? `搜索结果 · ${resultSummary}` : `当前召回仓库 · ${resultSummary}`}</p></div>
      <button type="button" className="icon" onClick={actions.startNew} title="新建召回条目"><Plus size={17} /></button>
    </div>
    <form className="mem-lite-search" onSubmit={actions.searchMemories}>
      <Search size={15} />
      <input aria-label="搜索召回内容" value={state.query} onChange={(event) => actions.setQuery(event.target.value)} placeholder="搜索召回内容" />
      <button type="submit">搜索</button>
    </form>
    <div className="mem-lite-files">
      {state.loading ? <p className="mem-lite-empty">正在读取召回内容…</p> : fileEntries.length === 0 ? <p className="mem-lite-empty">没有匹配的召回文件。</p> : fileGroups.map((group) => (
        <section className="mem-lite-folder" key={group.folderPath || '__root__'}>
          <div className="mem-lite-folder-head">
            <Folder size={15} />
            <span><strong>{group.folderName}</strong><small>{group.folderPath || '仓库根目录'}</small></span>
            <em>{group.files.length} 个 · {formatBytes(group.totalSizeBytes)}</em>
          </div>
          <div className="mem-lite-folder-files">
            {group.files.map((entry) => (
              <button type="button" key={entry.path} className={state.current?.path === entry.path ? 'active' : ''} onClick={() => actions.openRecall(entry.path)}>
                <FileText size={16} />
                <span><strong>{nameOf(entry.path)}</strong><small>{entry.path}</small></span>
                <em>{formatBytes(entry.size_bytes)}</em>
              </button>
            ))}
          </div>
        </section>
      ))}
    </div>
  </aside>;
}
