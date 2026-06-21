import { useEffect } from 'react'
import { useSettings } from '@/contexts/settings-context'
import { absoluteAssetUrl, upsertJsonLd, upsertLink, upsertMeta } from '@/lib/site-seo'

interface Props {
  /** Include WebSite / Organization JSON-LD (homepage). */
  structuredData?: boolean
  /** Optional page suffix, e.g. "Admin". */
  pageTitle?: string
}

export function SiteDocumentHead({ structuredData = false, pageTitle }: Props) {
  const { settings } = useSettings()

  useEffect(() => {
    const siteName = settings.site_name?.trim() || 'Looking Glass'
    const description = settings.site_description?.trim() || ''
    const logoUrl = settings.logo_path?.trim() ? absoluteAssetUrl(settings.logo_path) : ''

    const documentTitle = pageTitle
      ? `${pageTitle} | ${siteName}`
      : description
        ? `${siteName} — ${description}`
        : siteName
    document.title = documentTitle

    upsertMeta('name', 'description', description)
    upsertMeta('property', 'og:type', 'website')
    upsertMeta('property', 'og:title', siteName)
    upsertMeta('property', 'og:description', description)
    upsertMeta('property', 'og:site_name', siteName)
    upsertMeta('property', 'og:url', window.location.origin)
    upsertMeta('name', 'twitter:card', logoUrl ? 'summary' : 'summary')
    upsertMeta('name', 'twitter:title', siteName)
    upsertMeta('name', 'twitter:description', description)

    if (logoUrl) {
      upsertMeta('property', 'og:image', logoUrl)
      upsertMeta('name', 'twitter:image', logoUrl)
      upsertLink('apple-touch-icon', logoUrl)
    }

    if (structuredData) {
      upsertJsonLd('hopstat-site-jsonld', {
        '@context': 'https://schema.org',
        '@type': 'WebSite',
        name: siteName,
        description: description || undefined,
        url: window.location.origin,
        publisher: logoUrl
          ? {
              '@type': 'Organization',
              name: siteName,
              logo: {
                '@type': 'ImageObject',
                url: logoUrl,
              },
            }
          : {
              '@type': 'Organization',
              name: siteName,
            },
      })
    } else {
      upsertJsonLd('hopstat-site-jsonld', null)
    }
  }, [settings.site_name, settings.site_description, settings.logo_path, structuredData, pageTitle])

  return null
}
