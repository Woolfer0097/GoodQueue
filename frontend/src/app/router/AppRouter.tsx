import { Route, Routes } from 'react-router';

import { CatalogPage } from '@/pages/catalog';
import { CheckoutPage } from '@/pages/checkout';
import { NotFoundPage } from '@/pages/not-found';
import { ProductPage } from '@/pages/product';
import { QueuePage } from '@/pages/queue';
import { ReservationPage } from '@/pages/reservation';
import { ResultPage } from '@/pages/result';

import { AppLayout } from '../layouts/AppLayout';

export function AppRouter() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={<CatalogPage />} />
        <Route path="products/:productId">
          <Route index element={<ProductPage />} />
          <Route path="queue" element={<QueuePage />} />
          <Route path="reservation" element={<ReservationPage />} />
          <Route path="checkout" element={<CheckoutPage />} />
          <Route path="result" element={<ResultPage />} />
        </Route>
        <Route path="*" element={<NotFoundPage />} />
      </Route>
    </Routes>
  );
}
