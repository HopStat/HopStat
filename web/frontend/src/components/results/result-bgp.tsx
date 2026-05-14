import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { useI18n } from '@/contexts/i18n-context'
import type { BGPResult } from '@/types/domain'

interface Props { result: BGPResult }

export function ResultBGP({ result }: Props) {
  const { t } = useI18n()
  return (
    <div className="space-y-3">
      {(result.routes ?? []).map((route, i) => (
        <Card key={i}>
          <CardHeader className="pb-3">
            <CardTitle className="text-base font-mono">{route.prefix}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-2 text-sm">
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
              {route.next_hop && <div><span className="text-muted-foreground">{t('result.next_hop')}:</span> <span className="font-mono">{route.next_hop}</span></div>}
              {route.local_pref > 0 && <div><span className="text-muted-foreground">{t('result.local_pref')}:</span> {route.local_pref}</div>}
              {route.med > 0 && <div><span className="text-muted-foreground">{t('result.med')}:</span> {route.med}</div>}
              {route.origin && <div><span className="text-muted-foreground">{t('result.origin')}:</span> {route.origin}</div>}
            </div>
            {(route.as_path ?? []).length > 0 && (
              <div className="flex items-center gap-1 flex-wrap">
                <span className="text-muted-foreground text-xs mr-1">{t('result.as_path')}:</span>
                {(route.as_path ?? []).map((asn, j) => <Badge key={j} variant="info" className="text-xs">AS{asn}</Badge>)}
              </div>
            )}
            {(route.communities ?? []).length > 0 && (
              <div className="flex items-center gap-1 flex-wrap">
                <span className="text-muted-foreground text-xs mr-1">{t('result.communities')}:</span>
                {(route.communities ?? []).map((c, j) => <Badge key={j} variant="secondary" className="text-xs font-mono">{c}</Badge>)}
              </div>
            )}
          </CardContent>
        </Card>
      ))}
    </div>
  )
}
