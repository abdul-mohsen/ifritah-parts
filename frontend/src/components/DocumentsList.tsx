import type { Document } from '../types';

interface Props {
  documents: Document[];
}

// getDocIcon returns a simple text icon based on the TecDoc docType value.
function getDocIcon(docType?: string): string {
  const t = (docType || '').toLowerCase();
  if (t.includes('pdf')) return '📄';
  if (t.includes('mounting') || t.includes('install')) return '🔧';
  if (t.includes('safety') || t.includes('datasheet')) return '⚠️';
  if (t.includes('diagram') || t.includes('technical')) return '📐';
  if (t.includes('certification')) return '✅';
  return '📎';
}

/**
 * DocumentsList renders the list of TecDoc-sourced documents for an article.
 * Each document links to the TecDoc CDN URL and shows the type and language.
 *
 * IMPORTANT: per BUGS.md ("Rejected for ingestion: dealer pages, retail diagrams,
 * marketing/service portal media without a license") these rows are sourced ONLY
 * from the TecDoc articledocs table (LicensedSource="tecdoc:articledocs").
 * This component MUST NOT render documents from arbitrary web scraping.
 */
export function DocumentsList({ documents }: Props) {
  if (!documents || documents.length === 0) return null;

  return (
    <div style={{ marginTop: '8px' }}>
      <div style={{ fontSize: '0.75rem', fontWeight: 600, color: '#374151', marginBottom: '4px' }}>
        Documents & Manuals
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
        {documents.map((doc, i) => (
          <a
            key={i}
            href={doc.url}
            target="_blank"
            rel="noopener noreferrer"
            title={`${doc.docType || 'Document'} — ${doc.licensedSource || 'TecDoc'}`}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: '6px',
              padding: '4px 8px',
              borderRadius: '6px',
              backgroundColor: '#f8fafc',
              border: '1px solid #e2e8f0',
              fontSize: '0.78rem',
              color: '#1d4ed8',
              textDecoration: 'none',
              overflow: 'hidden',
            }}
          >
            <span style={{ flexShrink: 0 }}>{getDocIcon(doc.docType)}</span>
            <span style={{
              flexShrink: 1,
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}>
              {doc.fileName || doc.docType || 'Document'}
            </span>
            {doc.language && (
              <span style={{
                marginLeft: 'auto',
                flexShrink: 0,
                fontSize: '0.7rem',
                color: '#6b7280',
                backgroundColor: '#f1f5f9',
                padding: '0 4px',
                borderRadius: '3px',
              }}>
                {doc.language.toUpperCase()}
              </span>
            )}
          </a>
        ))}
      </div>
    </div>
  );
}
