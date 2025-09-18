import React from 'react';
import { AlertTriangle, Info, AlertCircle, CheckCircle } from 'lucide-react';

interface ConfirmDialogProps {
  isOpen: boolean;
  title: string;
  message: string;
  variant?: 'info' | 'warning' | 'danger' | 'success';
  confirmText?: string;
  cancelText?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export default function ConfirmDialog({
  isOpen,
  title,
  message,
  variant = 'info',
  confirmText = 'Yes',
  cancelText = 'No',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!isOpen) return null;

  const iconMap = {
    info: <Info className="h-6 w-6 text-blue-400" />,
    warning: <AlertTriangle className="h-6 w-6 text-yellow-400" />,
    danger: <AlertCircle className="h-6 w-6 text-red-400" />,
    success: <CheckCircle className="h-6 w-6 text-green-400" />,
  };

  const buttonColorMap = {
    info: 'bg-blue-600 hover:bg-blue-700',
    warning: 'bg-yellow-600 hover:bg-yellow-700',
    danger: 'bg-red-600 hover:bg-red-700',
    success: 'bg-green-600 hover:bg-green-700',
  };

  return (
    <div className="fixed inset-0 z-50 overflow-y-auto">
      {/* Backdrop */}
      <div 
        className="fixed inset-0 bg-black/60 backdrop-blur-sm transition-opacity"
        onClick={onCancel}
      />
      
      {/* Dialog */}
      <div className="flex min-h-full items-center justify-center p-4">
        <div className="relative bg-slate-900 rounded-lg border border-slate-800 shadow-2xl max-w-md w-full transform transition-all">
          {/* Header */}
          <div className="flex items-center gap-3 px-6 pt-6 pb-4">
            {iconMap[variant]}
            <h3 className="text-lg font-semibold text-white">
              {title}
            </h3>
          </div>
          
          {/* Body */}
          <div className="px-6 pb-6">
            <p className="text-slate-300 whitespace-pre-wrap">
              {message}
            </p>
          </div>
          
          {/* Footer */}
          <div className="flex gap-3 px-6 pb-6">
            <button
              onClick={onCancel}
              className="flex-1 px-4 py-2 bg-slate-800 hover:bg-slate-700 text-slate-300 rounded-md transition-colors font-medium"
            >
              {cancelText}
            </button>
            <button
              onClick={onConfirm}
              className={`flex-1 px-4 py-2 text-white rounded-md transition-colors font-medium ${buttonColorMap[variant]}`}
            >
              {confirmText}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}