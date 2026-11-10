/**
 * NotFoundPage - 404 error page for missing agents or resources.
 *
 * Features:
 * - User-friendly error message
 * - Actionable next steps
 * - Navigation back to home
 * - Page transition animation
 */

import React from 'react';
import { useNavigate } from 'react-router-dom';
import { PageTransition } from '@components/ui/PageTransition';
import { Button } from '@components/ui/Button';
import { ChevronRight, Compass } from 'lucide-react';

interface NotFoundPageProps {
  /** Type of resource not found */
  resourceType?: 'agent' | 'service' | 'page';
  /** Resource identifier (optional) */
  resourceId?: string;
}

/**
 * NotFoundPage displays a 404 error with helpful navigation.
 */
export function NotFoundPage({
  resourceType = 'page',
  resourceId,
}: NotFoundPageProps) {
  const navigate = useNavigate();

  // Resource-specific messages
  const messages = {
    agent: {
      title: 'Agent Not Found',
      description: resourceId
        ? `The agent "${resourceId}" is no longer available or you don't have access to it.`
        : 'The requested agent was not found.',
      suggestions: [
        'The agent may have been removed or deactivated',
        'You may not have permission to access this agent',
        'The agent ID in the URL may be incorrect',
      ],
    },
    service: {
      title: 'Service Not Found',
      description: 'The requested service is not available.',
      suggestions: [
        'The service may have been removed',
        'The service may be temporarily unavailable',
      ],
    },
    page: {
      title: 'Page Not Found',
      description: 'The page you are looking for does not exist.',
      suggestions: [
        'The URL may be incorrect',
        'The page may have been moved or deleted',
      ],
    },
  };

  const message = messages[resourceType];

  return (
    <div className="min-h-screen flex items-center justify-center bg-neutral-50 px-4 py-8">
      <PageTransition>
        <div className="max-w-2xl w-full text-center">
          {/* Error icon */}
          <div className="inline-flex items-center justify-center w-24 h-24 bg-neutral-200 rounded-full mb-6">
            <Compass
              className="w-12 h-12 text-neutral-400"
              aria-hidden="true"
              strokeWidth={1.5}
            />
          </div>

          {/* Error message */}
          <h1 className="text-4xl font-medium text-neutral-900 mb-3">
            {message.title}
          </h1>
          <p className="text-xl text-neutral-600 mb-8">{message.description}</p>

          {/* Suggestions */}
          <div className="bg-white rounded-lg shadow-sm border border-neutral-200 p-6 mb-8 text-left">
            <h2 className="text-sm font-medium text-neutral-900 mb-3">
              Possible reasons:
            </h2>
            <ul className="space-y-2">
              {message.suggestions.map((suggestion, index) => (
                <li
                  key={index}
                  className="flex items-start gap-2 text-sm text-neutral-600"
                >
                  <ChevronRight
                    className="w-5 h-5 text-neutral-400 flex-shrink-0 mt-0.5"
                    aria-hidden="true"
                    strokeWidth={1.5}
                  />
                  {suggestion}
                </li>
              ))}
            </ul>
          </div>

          {/* Actions */}
          <div className="flex flex-col-reverse sm:flex-row items-center justify-center gap-4">
            <Button variant="outline" onClick={() => navigate(-1)}>
              Go Back
            </Button>
            <Button variant="primary" onClick={() => navigate('/')}>
              Go to Home
            </Button>
          </div>

          {/* Help text */}
          <p className="mt-8 text-sm text-neutral-500">
            If you believe this is an error, please contact support.
          </p>
        </div>
      </PageTransition>
    </div>
  );
}

export default NotFoundPage;
