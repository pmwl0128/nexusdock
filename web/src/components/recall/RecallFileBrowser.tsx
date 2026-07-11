import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react';
import { ChevronDown, ChevronRight, FileText, Folder, FolderOpen, Plus, Search } from 'lucide-react';
import type { RecallEntry, RecallWorkspaceViewModel } from './types';
import { nameOf, normalizePath } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'fileEntries' | 'actions'>;

type RecallTreeNode = {
  name: string;
  path: string;
  folders: RecallTreeNode[];
  files: RecallEntry[];
  fileCount: number;
};

type MutableRecallTreeNode = RecallTreeNode & {
  folderMap: Map<string, MutableRecallTreeNode>;
  folders: MutableRecallTreeNode[];
};

function createFolderNode(name: string, path: string): MutableRecallTreeNode {
  return { name, path, folders: [], files: [], fileCount: 0, folderMap: new Map() };
}

function comparePath(a: string, b: string): number {
  return a.localeCompare(b, 'zh-CN');
}

function buildRecallTree(entries: RecallEntry[]): { root: RecallTreeNode; folderCount: number } {
  const root = createFolderNode('召回库', '');
  let folderCount = 0;

  for (const entry of entries) {
    const parts = normalizePath(entry.path).split('/').filter(Boolean);
    parts.pop();
    let parent = root;
    let currentPath = '';

    for (const part of parts) {
      currentPath = currentPath ? `${currentPath}/${part}` : part;
      let folder = parent.folderMap.get(part);
      if (!folder) {
        folder = createFolderNode(part, currentPath);
        parent.folderMap.set(part, folder);
        parent.folders.push(folder);
        folderCount += 1;
      }
      parent = folder;
    }
    parent.files.push(entry);
  }

  function finalize(node: MutableRecallTreeNode): RecallTreeNode {
    const childFolders = [...node.folders].sort((a, b) => comparePath(a.name, b.name));
    const files = [...node.files].sort((a, b) => comparePath(nameOf(a.path), nameOf(b.path)));

    let fileCount = files.length;
    const folders = childFolders.map((folder) => {
      const finalized = finalize(folder);
      fileCount += finalized.fileCount;
      return finalized;
    });

    return { name: node.name, path: node.path, folders, files, fileCount };
  }

  return { root: finalize(root), folderCount };
}

function treeIndent(depth: number): CSSProperties {
  return { '--tree-depth': depth } as CSSProperties;
}

function collectFolderPaths(node: RecallTreeNode): string[] {
  return node.folders.flatMap((folder) => [folder.path, ...collectFolderPaths(folder)]);
}

function defaultCollapsedFolders(root: RecallTreeNode): Set<string> {
  const collapsed = new Set<string>();
  function visit(folder: RecallTreeNode, depth: number) {
    if (depth >= 1) collapsed.add(folder.path);
    folder.folders.forEach((child) => visit(child, depth + 1));
  }
  root.folders.forEach((folder) => visit(folder, 0));
  return collapsed;
}

export default function RecallFileBrowser({ state, fileEntries, actions }: Props) {
  const { root, folderCount } = useMemo(() => buildRecallTree(fileEntries), [fileEntries]);
  const folderPaths = useMemo(() => collectFolderPaths(root), [root]);
  const [collapsedFolders, setCollapsedFolders] = useState<Set<string>>(() => new Set());
  const initializedTree = useRef(false);
  const allCollapsed = folderPaths.length > 0 && folderPaths.every((path) => collapsedFolders.has(path));
  const resultSummary = state.query
    ? `${fileEntries.length} 个结果 / ${folderCount} 个文件夹`
    : `${folderCount} 个文件夹 / ${fileEntries.length} 个文件`;

  useEffect(() => {
    if (initializedTree.current || fileEntries.length === 0) return;
    setCollapsedFolders(defaultCollapsedFolders(root));
    initializedTree.current = true;
  }, [fileEntries.length, root]);

  function toggleFolder(path: string) {
    setCollapsedFolders((current) => {
      const next = new Set(current);
      if (next.has(path)) next.delete(path); else next.add(path);
      return next;
    });
  }

  function toggleAllFolders() {
    setCollapsedFolders(allCollapsed ? new Set() : new Set(folderPaths));
  }

  function renderFiles(files: RecallEntry[], depth: number) {
    return files.map((entry) => (
      <button
        type="button"
        key={entry.path}
        className={`mem-lite-tree-row mem-lite-tree-file ${state.current?.path === entry.path ? 'active' : ''}`}
        style={treeIndent(depth)}
        role="treeitem"
        aria-level={depth + 1}
        aria-selected={state.current?.path === entry.path}
        title={entry.path}
        onClick={() => actions.openRecall(entry.path)}
      >
        <span className="mem-lite-tree-spacer" />
        <FileText size={15} />
        <span className="mem-lite-tree-label"><strong>{nameOf(entry.path)}</strong></span>
      </button>
    ));
  }

  function renderFolder(folder: RecallTreeNode, depth: number) {
    const collapsed = collapsedFolders.has(folder.path);
    const ToggleIcon = collapsed ? ChevronRight : ChevronDown;
    const FolderIcon = collapsed ? Folder : FolderOpen;

    return <div className="mem-lite-tree-folder" key={folder.path}>
      <button
        type="button"
        className="mem-lite-tree-row mem-lite-tree-folder-row"
        style={treeIndent(depth)}
        role="treeitem"
        aria-level={depth + 1}
        aria-expanded={!collapsed}
        aria-label={`${folder.name} 文件夹，${folder.fileCount} 个文件`}
        title={folder.path}
        onClick={() => toggleFolder(folder.path)}
      >
        <ToggleIcon size={14} />
        <FolderIcon size={16} />
        <span className="mem-lite-tree-label"><strong>{folder.name}</strong></span>
        <em>{folder.fileCount}</em>
      </button>
      {!collapsed && <div className="mem-lite-tree-children">
        {folder.folders.map((child) => renderFolder(child, depth + 1))}
        {renderFiles(folder.files, depth + 1)}
      </div>}
    </div>;
  }

  return <aside className="mem-lite-browser">
    <div className="mem-lite-panel-head">
      <div><h2>文件</h2><p>{state.query ? `搜索结果 · ${resultSummary}` : `文件管理器 · ${resultSummary}`}</p></div>
      <button type="button" className="icon" onClick={actions.startNew} title="新建召回条目" aria-label="新建召回条目"><Plus size={17} /></button>
    </div>
    <form className="mem-lite-search" onSubmit={actions.searchMemories}>
      <Search size={15} />
      <input aria-label="搜索召回内容" value={state.query} onChange={(event) => actions.setQuery(event.target.value)} placeholder="搜索召回内容" />
      <button type="submit">搜索</button>
    </form>
    <div className="mem-lite-tree-toolbar">
      <span><FolderOpen size={14} />召回目录</span>
      <button type="button" onClick={toggleAllFolders} disabled={folderPaths.length === 0}>{allCollapsed ? '展开全部' : '收起全部'}</button>
    </div>
    <div className="mem-lite-files mem-lite-tree" role="tree" aria-label="召回库文件树">
      {state.loading ? <p className="mem-lite-empty">正在读取召回内容…</p> : fileEntries.length === 0 ? <p className="mem-lite-empty">没有匹配的召回文件。</p> : <>
        {root.folders.map((folder) => renderFolder(folder, 0))}
        {renderFiles(root.files, 0)}
      </>}
    </div>
  </aside>;
}
