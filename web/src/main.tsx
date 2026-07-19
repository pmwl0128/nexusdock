import { createRoot } from 'react-dom/client';
import App from './App';
import { LoginPage } from './Auth';
import CredentialUpdatePage from './CredentialUpdatePage';
import './styles.css';
import './recall-nexus.css';
import './components/recall/recall-explorer.css';
import './theme.css';

const pathname = window.location.pathname;
const isLoginPage = pathname === '/login';
const isCredentialUpdatePage = pathname === '/change-password';

// iOS Safari 会使用 theme-color 渲染底部工具栏；工作台是浅色，认证页保留深色。
const themeColor = isLoginPage || isCredentialUpdatePage ? '#111425' : '#eff5f4';
document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.setAttribute('content', themeColor);

const page = isLoginPage
  ? <LoginPage />
  : isCredentialUpdatePage
    ? <CredentialUpdatePage />
    : <App />;

createRoot(document.getElementById('root')!).render(page);
