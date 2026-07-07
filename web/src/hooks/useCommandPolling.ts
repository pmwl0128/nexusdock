import { useCallback, useEffect, useReducer, useState } from 'react';
import { getCommand } from '../api/commands';
import type { DeviceCommand } from '../api/types';
import { TERMINAL_COMMAND_STATUSES } from '../components/devices/model';

type PollState = { command?: DeviceCommand; error: string; loading: boolean };
type PollAction =
  | { type: 'idle' }
  | { type: 'start'; seed?: DeviceCommand }
  | { type: 'success'; command: DeviceCommand }
  | { type: 'failure'; error: string };

function pollReducer(_state: PollState, action: PollAction): PollState {
  switch (action.type) {
    case 'idle':
      return { command: undefined, error: '', loading: false };
    case 'start':
      return { command: action.seed, error: '', loading: true };
    case 'success':
      return { command: action.command, error: '', loading: false };
    case 'failure':
      return { command: _state.command, error: action.error, loading: false };
  }
}

export function useCommandPolling(commandId?: string, seed?: DeviceCommand) {
  const [state, dispatch] = useReducer(pollReducer, { command: seed, error: '', loading: Boolean(commandId) });
  const [refreshToken, setRefreshToken] = useState(0);
  const refresh = useCallback(() => setRefreshToken((value) => value + 1), []);

  useEffect(() => {
    if (!commandId) {
      dispatch({ type: 'idle' });
      return;
    }
    let stopped = false;
    let timer = 0;
    let failures = 0;
    const startedAt = Date.now();
    const controller = new AbortController();
    dispatch({ type: 'start', seed });

    const poll = async () => {
      try {
        const next = await getCommand(commandId, controller.signal);
        if (stopped) return;
        dispatch({ type: 'success', command: next });
        failures = 0;
        if (TERMINAL_COMMAND_STATUSES.has(next.status)) return;
      } catch (reason) {
        if (stopped) return;
        failures += 1;
        dispatch({ type: 'failure', error: reason instanceof Error ? reason.message : '命令状态获取失败' });
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
  }, [commandId, refreshToken, seed]);

  return { ...state, refresh };
}
