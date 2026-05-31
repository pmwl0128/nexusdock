export function normalizePath(path: string): string {
  return String(path || '').replace(/^\/+|\/+$/g, '').replace(/\/+/g, '/');
}

export function fileName(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  return parts[parts.length - 1] || '';
}

export function parentPath(path: string): string {
  const parts = normalizePath(path).split('/').filter(Boolean);
  parts.pop();
  return parts.join('/');
}

export function joinPath(dir: string, name: string): string {
  dir = normalizePath(dir);
  name = fileName(name);
  return dir ? `${dir}/${name}` : name;
}
