import RecallWorkspaceView from './components/recall/RecallWorkspaceView';
import { useRecallWorkspaceController } from './components/recall/useRecallWorkspaceController';

export default function RecallWorkspace() {
  return <RecallWorkspaceView {...useRecallWorkspaceController()} />;
}
