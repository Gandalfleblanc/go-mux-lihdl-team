import '@fontsource-variable/unbounded'
import '@fontsource-variable/hanken-grotesk'
import '@fontsource-variable/jetbrains-mono'
import './tailwind.css'
import './style.css'
import './app.css'
import './theme.css'
import { mount } from 'svelte'
import App from './App.svelte'

// Attend les polices avant le premier rendu : sinon le texte s'affiche dans
// une police de secours puis se redimensionne (les cartes « sautent »).
const fontsReady = Promise.all([
  document.fonts.load('600 16px "Unbounded Variable"'),
  document.fonts.load('400 13px "Hanken Grotesk Variable"'),
  document.fonts.load('600 13px "Hanken Grotesk Variable"'),
  document.fonts.load('400 12px "JetBrains Mono Variable"'),
]).catch(() => {});
const timeout = new Promise((r) => setTimeout(r, 600));

Promise.race([fontsReady, timeout]).then(() => {
  mount(App, { target: document.getElementById('app') });
});
