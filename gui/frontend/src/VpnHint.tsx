import { useI18n } from './i18n/react';

/**
 * A small "VPN?" tag for a site whose answers were a sign it might block the
 * network it was reached from (State.vpn, SiteBar.vpn): "region" or "dns".
 * It only ever says *might*; nothing for "".
 */
export function VpnHint({ reason }: { reason: string }) {
  const { t } = useI18n();
  if (reason !== 'region' && reason !== 'dns') {
    return null;
  }
  const hint = reason === 'region' ? t('common.vpn.region') : t('common.vpn.dns');
  return (
    <span className="vpn-hint" title={hint} aria-label={hint} role="note">
      {t('common.vpn.badge')}
    </span>
  );
}
