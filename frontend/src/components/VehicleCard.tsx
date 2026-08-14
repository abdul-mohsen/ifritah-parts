import type { NHTSAVehicle, Vehicle } from '../types';

interface Props {
  nhtsa: NHTSAVehicle;
  vehicle?: Vehicle;
}

export default function VehicleCard({ nhtsa, vehicle }: Props) {
  return (
    <div className="bg-white rounded-lg border border-gray-200 p-5 shadow-sm">
      <h2 className="text-xl font-bold mb-3">
        {nhtsa.modelYear} {nhtsa.make} {nhtsa.model}
      </h2>
      <dl className="grid grid-cols-2 md:grid-cols-4 gap-x-6 gap-y-2 text-sm">
        {nhtsa.bodyClass && (
          <>
            <dt className="text-gray-500">Body</dt>
            <dd className="font-medium">{nhtsa.bodyClass}</dd>
          </>
        )}
        {nhtsa.driveType && (
          <>
            <dt className="text-gray-500">Drive</dt>
            <dd className="font-medium">{nhtsa.driveType}</dd>
          </>
        )}
        {nhtsa.fuelType && (
          <>
            <dt className="text-gray-500">Fuel</dt>
            <dd className="font-medium">{nhtsa.fuelType}</dd>
          </>
        )}
        {nhtsa.engineDisplacementCC && (
          <>
            <dt className="text-gray-500">Engine</dt>
            <dd className="font-medium">
              {nhtsa.engineDisplacementCC}cc{' '}
              {nhtsa.engineNumberOfCylinders
                ? `${nhtsa.engineNumberOfCylinders}-cyl`
                : ''}
            </dd>
          </>
        )}
        {vehicle && (
          <>
            <dt className="text-gray-500">TecDoc ID</dt>
            <dd className="font-mono font-medium">{vehicle.linkageTargetId}</dd>
          </>
        )}
      </dl>
    </div>
  );
}
