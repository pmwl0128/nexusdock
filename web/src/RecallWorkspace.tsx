import RecallWorkspaceView from './components/recall/RecallWorkspaceView';
import { useRecallWorkspaceController } from './components/recall/useRecallWorkspaceController';
import './recall.css';

export default function RecallWorkspace({ refreshToken }: { refreshToken: number }) {
  return <RecallWorkspaceView {...useRecallWorkspaceController(refreshToken)} />;
}
