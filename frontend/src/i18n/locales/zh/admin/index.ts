import overview from './overview'
import channels from './channels'
import accounts from './accounts'
import resources from './resources'
import ops from './ops'
import settings from './settings'
import tokenAnalysis from './tokenAnalysis'
import requestIntercept from './requestIntercept'
import promptMetrics from './promptMetrics'

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
}
