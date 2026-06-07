import { api } from './client';
import type {
  DeviceSnapshot,
  EnrollmentTokenCreateRequest,
  EnrollmentTokenCreateResponse,
  ItemsResponse,
  NexusDevice,
} from './types';

export function listDevices(signal?: AbortSignal): Promise<ItemsResponse<NexusDevice>> {
  return api('/v1/devices', { signal });
}

export function getDevice(deviceId: string, signal?: AbortSignal): Promise<DeviceSnapshot> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}`, { signal });
}

export function createEnrollmentToken(request: EnrollmentTokenCreateRequest): Promise<EnrollmentTokenCreateResponse> {
  return api('/v1/devices/enrollment-tokens', { method: 'POST', body: JSON.stringify(request) });
}

export function approveDevice(deviceId: string): Promise<void> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}/approve`, { method: 'POST' });
}

export function revokeDevice(deviceId: string, reason: string): Promise<void> {
  return api(`/v1/devices/${encodeURIComponent(deviceId)}/revoke`, {
    method: 'POST',
    body: JSON.stringify({ reason }),
  });
}
