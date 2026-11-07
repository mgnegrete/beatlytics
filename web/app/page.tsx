'use client';

import { useEffect, useState } from 'react';
import { useRouter } from 'next/navigation';
import { getMe, type Me } from '@/lib/api';
import BeatlyticsDashboard from '@/components/BeatlyticsDashboard';

export default function Page() {
  const router = useRouter();
  const [me, setMe] = useState<Me | null>(null);
  const [checking, setChecking] = useState(true);

  useEffect(() => {
    getMe().then((user) => {
      if (!user) {
        router.push('/login');
      } else {
        setMe(user);
        setChecking(false);
      }
    });
  }, [router]);

  if (checking) return null;
  return <BeatlyticsDashboard user={me!} />;
}
