export { getProduct, getProductAlternatives, getProducts } from './api/product.api';
export {
  useProductAlternativesQuery,
  useProductQuery,
  useProductsQuery,
} from './api/product.queries';
export { productQueryKeys } from './api/product.query-keys';
export { getProductAvailability, type ProductAvailability } from './model/product.availability';
export {
  formatProductCategory,
  formatProductPrice,
  getProductAvailabilityPresentation,
  PRODUCT_IMAGE_PLACEHOLDER,
} from './model/product.presentation';
export {
  type Product,
  type ProductAlternative,
  productAlternativesSchema,
  productListSchema,
  productSchema,
} from './model/product.schema';
export { ProductAvailabilityBadge } from './ui/ProductAvailabilityBadge';
export { ProductCard, type ProductCardUserStatus } from './ui/ProductCard';
export { ProductCardSkeleton } from './ui/ProductCardSkeleton';
