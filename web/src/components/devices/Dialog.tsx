import { useEffect, useRef, type ReactNode } from 'react';
import { X } from 'lucide-react';

export default function Dialog({ title, description, children, onClose, wide = false }: {
  title: string;
  description?: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
}) {
  const panelRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    const panel = panelRef.current;
    panel?.querySelector<HTMLElement>('button, input, select, textarea')?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
      if (event.key !== 'Tab' || !panel) return;
      const items = Array.from(panel.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled)'));
      if (items.length === 0) return;
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', onKeyDown);
    document.body.classList.add('nexus-modal-open');
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.body.classList.remove('nexus-modal-open');
      previous?.focus();
    };
  }, [onClose]);

  return (
    <div className="nx-dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.currentTarget === event.target) onClose(); }}>
      <div ref={panelRef} className={`nx-dialog ${wide ? 'is-wide' : ''}`} role="dialog" aria-modal="true" aria-labelledby="nx-dialog-title">
        <header>
          <div><h2 id="nx-dialog-title">{title}</h2>{description && <p>{description}</p>}</div>
          <button type="button" className="nx-icon-button" aria-label="关闭" onClick={onClose}><X size={19} /></button>
        </header>
        <div className="nx-dialog-body">{children}</div>
      </div>
    </div>
  );
}
