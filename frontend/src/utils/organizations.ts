export function formatOrganizationUsageOrganization(value: string, otherLabel = '其他'): string {
  if (value === 'xunyou' || value === 'xunyou.com') return '迅游'
  if (value === 'wsdashi' || value === 'wsdashi.com') return '速宝'
  return otherLabel
}
