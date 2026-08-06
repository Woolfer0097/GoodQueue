import { Route, Routes } from 'react-router';

import { AppLayout } from './layouts/AppLayout.tsx';

function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route index element={null} />
        <Route path="products/:productId">
          <Route index element={null} />
          <Route path="queue" element={null} />
          <Route path="reservation" element={null} />
          <Route path="checkout" element={null} />
          <Route path="result" element={null} />
        </Route>
        <Route path="*" element={null} />
      </Route>
    </Routes>
  );
}

export default App;
