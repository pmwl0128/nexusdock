import { api } from './client';
import type { DeviceCommand, DeviceCommandCreateRequest, ItemsResponse } from './types';

export function listDeviceCommands(deviceId: string, signal?: AbortSignal): Promise<ItemsResponse<DeviceCommand>> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}/commands`, { signal });
}

export function getCommand(commandId: string, signal?: AbortSignal): Promise<DeviceCommand> {
  return api(`/v1/commands/${encodeURIComponent(commandId)}`, { signal });
}

export function createDeviceCommand(deviceId: string, request: DeviceCommandCreateRequest): Promise<DeviceCommand> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}/commands`, {
    method: 'POST',
    body: JSON.stringify(request),
  });
}
