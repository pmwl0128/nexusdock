import type { CommandStatus, DeviceStatus } from '../../api/types';
import { COMMAND_STATUS_LABELS, STATUS_LABELS } from './model';

export function DeviceStatusBadge({ status }: { status: DeviceStatus }) {
  return <span className={`nx-status nx-status-${status}`}><span />{STATUS_LABELS[status]}</span>;
}

export function CommandStatusBadge({ status }: { status: CommandStatus }) {
  return <span className={`nx-status nx-status-${status}`}><span />{COMMAND_STATUS_LABELS[status]}</span>;
}
