import { FileText, Pencil, Plus, Save, Trash2 } from 'lucide-react';
import type { RecallWorkspaceViewModel } from './types';
import { nameOf } from './utils';

type Props = Pick<RecallWorkspaceViewModel, 'state' | 'hasUnsavedChanges' | 'editorRef' | 'actions'>;

export default function RecallEditor({ state, hasUnsavedChanges, editorRef, actions }: Props) {
  const title = state.editing
    ? state.creating ? '新建召回条目' : '编辑召回条目'
    : state.current ? nameOf(state.current.path) : '选择一条召回内容';
  const subtitle = state.editing ? state.draftPath : state.current?.path || '从左侧文件列表打开，或新建一条召回内容。';
  return <article className="mem-lite-editor" ref={editorRef}>
    <div className="mem-lite-panel-head">
      <div><h2>{title}</h2><p>{subtitle}</p></div>
      <button className="mem-lite-mobile-back" type="button" onClick={actions.backToFileList}>返回文件</button>
      <div className="mem-lite-editor-actions">
        {!state.editing && state.current && <button type="button" onClick={actions.startEdit}><Pencil size={15} />编辑</button>}
        {!state.editing && state.current && <button type="button" onClick={actions.requestMove}>移动</button>}
        {!state.editing && state.current && <button type="button" className="danger" onClick={actions.requestDelete}><Trash2 size={15} />删除</button>}
        {state.editing && <button type="button" onClick={actions.cancelEdit}>取消</button>}
        {state.editing && <button type="button" className="primary" onClick={actions.saveRecall} disabled={state.busy || !hasUnsavedChanges}><Save size={15} />保存</button>}
      </div>
    </div>
    {state.editing ? (
      <div className="mem-lite-edit-body">
        <label><span>路径</span><input value={state.draftPath} onChange={(event) => actions.setDraftPath(event.target.value)} disabled={!state.creating} /></label>
        <label className="content"><span>内容</span><textarea value={state.draftContent} onChange={(event) => actions.setDraftContent(event.target.value)} spellCheck={false} /></label>
        <small>{state.draftContent.length.toLocaleString()} 字符 · 草稿自动保存在当前浏览器会话</small>
      </div>
    ) : state.current ? (
      <pre className="mem-lite-preview">{state.current.content}</pre>
    ) : (
      <div className="mem-lite-empty large"><FileText size={28} /><strong>没有打开的召回内容</strong><span>选择文件或创建一条新召回条目。</span><button type="button" className="primary" onClick={actions.startNew}><Plus size={15} />新建召回条目</button></div>
    )}
  </article>;
}
