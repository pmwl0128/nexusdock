import { useCallback, useEffect, useState } from 'react';
import { getCommand } from '../api/commands';
import type { DeviceCommand } from '../api/types';
import { TERMINAL_COMMAND_STATUSES } from '../components/devices/model';

export function useCommandPolling(commandId?: string, seed?: DeviceCommand) {
  const [command, setCommand] = useState<DeviceCommand | undefined>(seed);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(Boolean(commandId));
  const [refreshToken, setRefreshToken] = useState(0);
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);

  useEffect(() => {
    if (!commandId) {
      setCommand(undefined);
      setLoading(false);
      setError('');
      return;
    }
    let stopped = false;
    let timer = 0;
    let failures = 0;
    const startedAt = Date.now();
    const controller = new AbortController();
    setCommand(seed);
    setLoading(true);
    setError('');

    const poll = async () => {
      try {
        const next = await getCommand(commandId, controller.signal);
        if (stopped) return;
        setCommand(next);
        setError('');
        setLoading(false);
        failures = 0;
        if (TERMINAL_COMMAND_STATUSES.has(next.status)) return;
      } catch (reason) {
        if (stopped) return;
        failures += 1;
        setError(reason instanceof Error ? reason.message : '命令状态获取失败');
        setLoading(false);
      }
      const age = Date.now() - startedAt;
      const base = document.hidden ? 15000 : age < 30000 ? 2000 : 5000;
      const delay = Math.min(30000, base * Math.max(1, 2 ** Math.max(0, failures - 1)));
      timer = window.setTimeout(() => void poll(), delay);
    };

    const onVisibility = () => {
      if (!document.hidden) {
        window.clearTimeout(timer);
        void poll();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    void poll();
    return () => {
      stopped = true;
      controller.abort();
      window.clearTimeout(timer);
      document.removeEventListener('visibilitychange', onVisibility);
    };
  }, [commandId, refreshToken]);

  return { command, error, loading, refresh };
}
