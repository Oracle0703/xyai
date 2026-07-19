import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import tokenAnalysis from './tokenAnalysis'
import requestIntercept from './requestIntercept'
import promptMetrics from './promptMetrics'
import organizationUsage from './organizationUsage'
import audit from './audit'
import promptAudit from './promptAudit'

export default {
  ...overview,
  ...channels,
  ...accounts,
  ...resources,
  ...ops,
  ...settings,
  ...tokenAnalysis,
  ...requestIntercept,
  ...promptMetrics,
  ...organizationUsage,
  ...audit,
  ...promptAudit,
}
