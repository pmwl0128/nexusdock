import RecallWorkspaceView from './components/recall/RecallWorkspaceView';
import { useRecallWorkspaceController } from './components/recall/useRecallWorkspaceController';

export default function RecallWorkspace({ refreshToken }: { refreshToken: number }) {
  return <RecallWorkspaceView {...useRecallWorkspaceController(refreshToken)} />;
}
