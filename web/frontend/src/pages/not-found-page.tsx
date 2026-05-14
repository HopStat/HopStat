import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Home } from 'lucide-react'
import { useI18n } from '@/contexts/i18n-context'

export function NotFoundPage() {
  const { t } = useI18n()
  return (
    <div className="min-h-screen flex items-center justify-center bg-background">
      <div className="text-center space-y-4">
        <h1 className="text-6xl font-bold text-muted-foreground">404</h1>
        <p className="text-lg text-muted-foreground">{t('not_found.title')}</p>
        <Link to="/"><Button><Home className="w-4 h-4 mr-2" /> {t('not_found.go_home')}</Button></Link>
      </div>
    </div>
  )
}
