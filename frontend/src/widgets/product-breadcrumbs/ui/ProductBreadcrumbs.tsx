import { Anchor, Breadcrumbs, Text } from '@mantine/core';
import { IconChevronRight } from '@tabler/icons-react';
import { generatePath, Link } from 'react-router';

interface ProductBreadcrumbsProps {
  currentPage?: string;
  productId: string;
  productTitle?: string;
}

export function ProductBreadcrumbs({
  currentPage,
  productId,
  productTitle,
}: ProductBreadcrumbsProps) {
  const productLabel = productTitle ?? 'Товар';
  const productPath = generatePath('/products/:productId', { productId });

  return (
    <nav aria-label="Хлебные крошки">
      <Breadcrumbs
        separator={
          <IconChevronRight
            aria-hidden
            color="var(--mantine-color-dimmed)"
            size={14}
            stroke={1.5}
          />
        }
        separatorMargin={2}
        styles={{
          breadcrumb: { minWidth: 0 },
          root: { flexWrap: 'nowrap', overflow: 'hidden' },
          separator: { flex: '0 0 auto' },
        }}
      >
        <Anchor c="dimmed" component={Link} size="sm" to="/" underline="hover">
          Каталог
        </Anchor>
        {currentPage ? (
          <Anchor
            c="dimmed"
            component={Link}
            maw="min(22.5rem, 45vw)"
            size="sm"
            title={productLabel}
            to={productPath}
            truncate
            underline="hover"
          >
            {productLabel}
          </Anchor>
        ) : (
          <Text
            aria-current="page"
            c="dimmed"
            component="span"
            maw="min(30rem, 60vw)"
            size="sm"
            title={productLabel}
            truncate
          >
            {productLabel}
          </Text>
        )}
        {currentPage && (
          <Text aria-current="page" c="dimmed" component="span" size="sm" textWrap="nowrap">
            {currentPage}
          </Text>
        )}
      </Breadcrumbs>
    </nav>
  );
}
