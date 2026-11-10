/**
 * ErrorPage - 404 Not Found page for unmatched routes.
 */

import React from 'react';
import { Link } from 'react-router-dom';
import { AppLayout } from '@components/layout/AppLayout';

export function ErrorPage() {
  return (
    <AppLayout>
      <div className="flex flex-col items-center justify-center py-16">
        <div className="text-center">
          <h1 className="text-6xl font-medium text-neutral-900 mb-4">404</h1>
          <h2 className="text-2xl font-medium text-neutral-700 mb-2">
            Page Not Found
          </h2>
          <p className="text-neutral-600 mb-8 max-w-md">
            The page you&apos;re looking for doesn&apos;t exist or has been
            moved.
          </p>
          <Link
            to="/"
            className="inline-block bg-accent text-white px-6 py-3 rounded-lg hover:bg-accent-hover transition-colors font-medium"
          >
            Go to Home
          </Link>
        </div>
      </div>
    </AppLayout>
  );
}

export default ErrorPage;
