import {hideElem, showElem, toggleElem} from '../utils/dom.ts';
import {sanitizeRepoName} from './repo-common.ts';

const service = document.querySelector<HTMLInputElement>('#service_type');
const user = document.querySelector<HTMLInputElement>('#auth_username');
const pass = document.querySelector<HTMLInputElement>('#auth_password');
const token = document.querySelector<HTMLInputElement>('#auth_token');
const mirror = document.querySelector<HTMLInputElement>('#mirror');
const lfs = document.querySelector<HTMLInputElement>('#lfs');
const lfsSettings = document.querySelector<HTMLElement>('#lfs_settings')!;
const lfsEndpoint = document.querySelector<HTMLElement>('#lfs_endpoint')!;
const items = document.querySelectorAll<HTMLInputElement>('#migrate_items input[type=checkbox]');
const syncItems = document.querySelector<HTMLElement>('#migrate_sync_items');

export function initRepoMigration() {
  checkAuth();
  setLFSSettingsVisibility();
  setSyncItemsVisibility();

  user?.addEventListener('input', () => {checkItems(false)});
  pass?.addEventListener('input', () => {checkItems(false)});
  token?.addEventListener('input', () => {checkItems(true)});
  mirror?.addEventListener('change', () => {checkItems(true); setSyncItemsVisibility()});
  document.querySelector('#lfs_settings_show')?.addEventListener('click', (e) => {
    e.preventDefault();
    e.stopPropagation();
    showElem(lfsEndpoint);
  });
  lfs?.addEventListener('change', setLFSSettingsVisibility);

  const elCloneAddr = document.querySelector<HTMLInputElement>('#clone_addr');
  const elRepoName = document.querySelector<HTMLInputElement>('#repo_name');
  if (elCloneAddr && elRepoName) {
    let repoNameChanged = false;
    elRepoName.addEventListener('input', () => {repoNameChanged = true});
    elCloneAddr.addEventListener('input', () => {
      if (repoNameChanged) return;
      let repoNameFromUrl = elCloneAddr.value.split(/[?#]/)[0];
      const parts = /^(.*\/)?((.+?)\/?)$/.exec(repoNameFromUrl);
      if (!parts || parts.length < 4) {
        elRepoName.value = '';
        return;
      }
      repoNameFromUrl = parts[3].split(/[?#]/)[0];
      elRepoName.value = sanitizeRepoName(repoNameFromUrl);
    });
  }
}

function checkAuth() {
  if (!service) return;
  const serviceType = Number(service.value);

  checkItems(serviceType !== 1);
}

function checkItems(tokenAuth: boolean) {
  let enableItems: boolean;
  if (tokenAuth) {
    enableItems = token?.value !== '';
  } else {
    enableItems = user?.value !== '' || pass?.value !== '';
  }
  const hasToken = enableItems && Number(service?.value) > 1;
  // One-time migration items: enabled when there's a token and NOT in mirror mode
  // (mirror mode hides them and shows the sync metadata section instead).
  for (const item of items) {
    item.disabled = !hasToken || Boolean(mirror?.checked);
  }
  // Sync metadata checkboxes: enabled when there's a token and IN mirror mode.
  const syncCheckboxes = syncItems?.querySelectorAll<HTMLInputElement>('input[type=checkbox]');
  if (syncCheckboxes) {
    for (const item of syncCheckboxes) {
      item.disabled = !hasToken;
    }
  }
}

function setLFSSettingsVisibility() {
  if (!lfs) return;
  const visible = lfs.checked;
  toggleElem(lfsSettings, visible);
  hideElem(lfsEndpoint);
}

function setSyncItemsVisibility() {
  if (!syncItems) return;
  const isMirror = Boolean(mirror?.checked);
  toggleElem(syncItems, isMirror);
  // When mirror mode is on, the one-time migration items (issues/PRs/labels/milestones/
  // releases) are irrelevant — the sync metadata options replace them. Hide the grayed-out
  // duplicates so the form shows one clear set of choices, not two.
  const migrateItems = document.querySelector<HTMLElement>('#migrate_items');
  if (migrateItems) toggleElem(migrateItems, !isMirror);
}
