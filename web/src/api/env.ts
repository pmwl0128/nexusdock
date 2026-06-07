import { api } from './client';
import type { DeviceCommand } from './types';

export type EnvActionRequest = {
  action: 'list' | 'inspect' | 'set' | 'delete' | 'verify' | 'migrate-from-agentdock-env';
  skill?: string;
  name?: string;
  kind?: 'plain' | 'secret';
  value?: string;
  operation?: string;
  env_file?: string;
};

export function createEnvAction(deviceId: string, request: EnvActionRequest): Promise<DeviceCommand> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}/env/actions`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
}
