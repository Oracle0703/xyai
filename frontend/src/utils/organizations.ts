export function formatOrganizationUsageOrganization(value: string, otherLabel = '其他'): string {
  if (value === 'xunyou' || value === 'xunyou.com') return '迅游'
  if (value === 'wsdashi' || value === 'wsdashi.com') return '速宝'
  return otherLabel
}

// Mirrors the backend OrganizationForEmail; the server stays authoritative.
export function organizationForEmail(email: string): 'xunyou' | 'wsdashi' | 'other' {
  const domain = email.split('@')[1]?.toLowerCase()
  if (domain === 'xunyou.com') return 'xunyou'
  if (domain === 'wsdashi.com') return 'wsdashi'
  return 'other'
}
