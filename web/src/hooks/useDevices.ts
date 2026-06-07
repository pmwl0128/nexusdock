import { useCallback, useEffect, useState } from 'react';
import { getDevice, listDevices } from '../api/devices';
import type { DeviceSnapshot } from '../api/types';

export function useDevices(refreshToken: number) {
  const [devices, setDevices] = useState<DeviceSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [generation, setGeneration] = useState(0);
  const refresh = useCallback(() => setGeneration((value) => value + 1), []);

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError('');
    void listDevices(controller.signal)
      .then(async ({ items }) => {
        const snapshots = await Promise.all(items.map(async (device) => {
          try {
            return await getDevice(device.id, controller.signal);
          } catch {
            return { device };
          }
        }));
        if (!controller.signal.aborted) setDevices(snapshots);
      })
      .catch((reason: unknown) => {
        if (!controller.signal.aborted) setError(reason instanceof Error ? reason.message : '设备列表加载失败');
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [refreshToken, generation]);

  return { devices, loading, error, refresh };
}
